// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type userID int64

// inRequest runs fn inside a routed request for target on /items/{id}.
func inRequest(t *testing.T, target string, fn func(c *Context)) {
	t.Helper()
	app := New()
	ran := false
	app.Get("/items/{id}", func(c *Context) error { fn(c); ran = true; return nil })
	app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	if !ran {
		t.Fatalf("handler did not run for %s", target)
	}
}

func TestParseValueTypes(t *testing.T) {
	check := func(name string, got, want any, err error) {
		t.Helper()
		if err != nil || got != want {
			t.Errorf("%s: got %v (%T) err=%v, want %v", name, got, got, err, want)
		}
	}
	i, err := parseValue[int]("-42")
	check("int", i, -42, err)
	i64, err := parseValue[int64]("9000000000")
	check("int64", i64, int64(9000000000), err)
	i32, err := parseValue[int32]("7")
	check("int32", i32, int32(7), err)
	u, err := parseValue[uint]("7")
	check("uint", u, uint(7), err)
	u64, err := parseValue[uint64]("7")
	check("uint64", u64, uint64(7), err)
	u32, err := parseValue[uint32]("7")
	check("uint32", u32, uint32(7), err)
	f, err := parseValue[float64]("1.5")
	check("float64", f, 1.5, err)
	f32, err := parseValue[float32]("1.5")
	check("float32", f32, float32(1.5), err)
	b, err := parseValue[bool]("true")
	check("bool", b, true, err)
	s, err := parseValue[string]("zinc")
	check("string", s, "zinc", err)

	// Types outside the fast path go through the struct-binding setters.
	id, err := parseValue[userID]("42")
	check("named int", id, userID(42), err)
	i8, err := parseValue[int8]("-8")
	check("int8", i8, int8(-8), err)
	ts, err := parseValue[time.Time]("2026-09-25T10:00:00Z")
	check("TextUnmarshaler", ts.Equal(time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)), true, err)
	p, err := parseValue[*int]("5")
	if err != nil || p == nil || *p != 5 {
		t.Errorf("pointer: %v %v", p, err)
	}

	for name, err := range map[string]error{
		"int overflow":  second(parseValue[int32]("99999999999")),
		"int syntax":    second(parseValue[int]("x")),
		"bool syntax":   second(parseValue[bool]("maybe")),
		"unsupported":   second(parseValue[struct{ A int }]("x")),
		"time syntax":   second(parseValue[time.Time]("yesterday")),
		"named syntax":  second(parseValue[userID]("x")),
		"int8 overflow": second(parseValue[int8]("300")),
	} {
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func second[T any](_ T, err error) error { return err }

func TestParamQueryAndFormAccessors(t *testing.T) {
	inRequest(t, "/items/42?page=3&flag=true&bad=x&empty=", func(c *Context) {
		if id, err := Param[int](c, "id"); err != nil || id != 42 {
			t.Errorf("Param = %d %v", id, err)
		}
		if id, err := Param[userID](c, "id"); err != nil || id != 42 {
			t.Errorf("Param[userID] = %d %v", id, err)
		}
		if page, err := Query[int](c, "page"); err != nil || page != 3 {
			t.Errorf("Query = %d %v", page, err)
		}
		if flag, err := Query[bool](c, "flag"); err != nil || !flag {
			t.Errorf("Query[bool] = %v %v", flag, err)
		}
		if got := QueryOr(c, "page", 1); got != 3 {
			t.Errorf("QueryOr present = %d", got)
		}
		if got := QueryOr(c, "limit", 20); got != 20 {
			t.Errorf("QueryOr missing = %d", got)
		}
		if got := QueryOr(c, "bad", 20); got != 20 {
			t.Errorf("QueryOr unparsable = %d", got)
		}
		// An empty value is present: strings return it, numbers fail to parse.
		if got, err := Query[string](c, "empty"); err != nil || got != "" {
			t.Errorf("Query empty string = %q %v", got, err)
		}

		var bindErr *BindError
		if _, err := Query[int](c, "bad"); !errors.As(err, &bindErr) ||
			bindErr.Source != "query" || bindErr.Name != "bad" || bindErr.Reason != "must be an integer" {
			t.Errorf("invalid query err = %#v", err)
		}
		if _, err := Query[int](c, "missing"); !errors.As(err, &bindErr) || bindErr.Reason != "is required" {
			t.Errorf("missing query err = %#v", err)
		}
		if _, err := Param[int](c, "nope"); !errors.As(err, &bindErr) || bindErr.Source != "path" {
			t.Errorf("missing param err = %#v", err)
		}
	})
}

// Returning an accessor error gives the client a 400 naming the field.
func TestAccessorErrorResponse(t *testing.T) {
	app := New()
	app.Get("/items/{id}", func(c *Context) error {
		id, err := Param[int](c, "id")
		if err != nil {
			return err
		}
		page, err := Query[int](c, "page")
		if err != nil {
			return err
		}
		return c.JSON(Map{"id": id, "page": page})
	})
	for target, want := range map[string]string{
		"/items/abc":        `{"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}` + "\n",
		"/items/1":          `{"error":{"status":400,"message":"invalid query parameter","fields":{"page":"is required"}}}` + "\n",
		"/items/1?page=two": `{"error":{"status":400,"message":"invalid query parameter","fields":{"page":"must be an integer"}}}` + "\n",
		"/items/1?page=2":   `{"id":1,"page":2}` + "\n",
	} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Body.String() != want {
			t.Errorf("%s: %d %q\nwant %q", target, rec.Code, rec.Body.String(), want)
		}
	}
}

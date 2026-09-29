// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type listQuery struct {
	Limit   int      `query:"limit" default:"20"`
	Sort    string   `query:"sort" default:"name"`
	Fields  []string `query:"fields" default:"id,name"`
	Page    *int     `query:"page" default:"1"`
	Region  string   `header:"X-Region" default:"eu"`
	Session string   `cookie:"session"`
	Theme   string   `cookie:"theme" default:"light"`
}

func TestBindDefaultsAndCookies(t *testing.T) {
	app := New()
	app.Get("/items", Typed(func(_ *Context, in listQuery) (listQuery, error) { return in, nil }))
	get := func(target string, cookies ...*http.Cookie) listQuery {
		t.Helper()
		req := httptest.NewRequest("GET", target, nil)
		req.Header.Set("X-Region", "")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", target, rec.Code, rec.Body)
		}
		var out listQuery
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	got := get("/items")
	if got.Limit != 20 || got.Sort != "name" || strings.Join(got.Fields, ",") != "id,name" || got.Page == nil || *got.Page != 1 || got.Theme != "light" {
		t.Fatalf("defaults: %+v", got)
	}
	// A header sent empty is still sent, so it replaces the default.
	if got.Region != "" {
		t.Fatalf("an empty header should bind: %q", got.Region)
	}

	got = get("/items?limit=5&fields=a&fields=b", &http.Cookie{Name: "session", Value: "abc"}, &http.Cookie{Name: "theme", Value: "dark"})
	if got.Limit != 5 || strings.Join(got.Fields, ",") != "a,b" || got.Session != "abc" || got.Theme != "dark" || got.Sort != "name" {
		t.Fatalf("request values: %+v", got)
	}
}

func TestBindDefaultsPerSource(t *testing.T) {
	app := New()
	app.Get("/", func(c *Context) error {
		var q listQuery
		if err := c.Bind().Query(&q); err != nil {
			return err
		}
		// Query applies the query fields' defaults only.
		if q.Limit != 20 || q.Region != "" || q.Theme != "" {
			t.Errorf("query defaults: %+v", q)
		}
		var ck listQuery
		if err := c.Bind().Cookie(&ck); err != nil {
			return err
		}
		if ck.Session != "s1" || ck.Theme != "light" || ck.Limit != 0 {
			t.Errorf("cookie binding: %+v", ck)
		}
		return c.NoContent()
	})
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "s1"})
	app.ServeHTTP(httptest.NewRecorder(), req)
}

func TestBindDefaultMustFitTheField(t *testing.T) {
	type bad struct {
		Limit int `query:"limit" default:"lots"`
	}
	mustPanicWith(t, `default tag "lots" on bad.Limit`, func() {
		New().Get("/", Typed(func(*Context, bad) (NoContent, error) { return NoContent{}, nil }))
	})
}

func TestBindCookieError(t *testing.T) {
	type in struct {
		N int `cookie:"n"`
	}
	app := New()
	app.Get("/", Typed(func(*Context, in) (NoContent, error) { return NoContent{}, nil }))
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "n", Value: "x"})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "cookie") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}

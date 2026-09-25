// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"
)

type userNotFound struct{ id string }

func (e userNotFound) Error() string { return "user " + e.id + " not found" }
func (userNotFound) StatusCode() int { return http.StatusNotFound }

type dbDown struct{}

func (dbDown) Error() string   { return "db: connection refused on 10.0.0.7" }
func (dbDown) StatusCode() int { return http.StatusServiceUnavailable }

type fieldErrors map[string]string

func (f fieldErrors) Error() string             { return "invalid fields" }
func (f fieldErrors) Fields() map[string]string { return f }

type stubValidator struct{ err error }

func (v stubValidator) Validate(any) error { return v.err }

func errorBodyOf(status int, message string) string {
	return fmt.Sprintf(`{"error":{"status":%d,"message":%q}}`+"\n", status, message)
}

func TestErrorConstructors(t *testing.T) {
	cases := []struct {
		err  *HTTPError
		code int
	}{
		{BadRequest("m"), 400}, {Unauthorized("m"), 401}, {Forbidden("m"), 403},
		{NotFound("m"), 404}, {Conflict("m"), 409}, {Gone("m"), 410},
		{UnprocessableEntity("m"), 422}, {TooManyRequests("m"), 429},
		{InternalServerError("m"), 500}, {ServiceUnavailable("m"), 503},
	}
	for _, tc := range cases {
		if tc.err.Code != tc.code || tc.err.Message != "m" {
			t.Errorf("got %d %q, want %d", tc.err.Code, tc.err.Message, tc.code)
		}
		if !errors.Is(tc.err, NewError(tc.code)) {
			t.Errorf("%d does not match its sentinel", tc.code)
		}
	}
	if err := NewError(418); err.Message != "" || err.Error() != "I'm a teapot" {
		t.Fatalf("NewError without message: %q", err.Error())
	}
	if err := NewError(418, "short and stout"); err.Error() != "short and stout" {
		t.Fatalf("NewError with message: %q", err.Error())
	}
	if NotFound("").Error() != "Not Found" {
		t.Fatal("empty message should fall back to the status text")
	}
}

func TestStatusCodeResolution(t *testing.T) {
	oversized := &BindError{Source: "body", Err: ErrRequestEntityTooLarge}
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"sentinel", ErrForbidden, 403},
		{"wrapped HTTP error", fmt.Errorf("load: %w", NotFound("x")), 404},
		{"domain StatusCoder", fmt.Errorf("load: %w", userNotFound{"7"}), 404},
		{"bind error", &BindError{Source: "query"}, 400},
		{"bind error caused by a 413", oversized, 413},
		{"validation error", &ValidationError{Err: errors.New("bad")}, 422},
		{"plain error", errors.New("boom"), 500},
		{"StatusCoder outside 4xx and 5xx", statusCoderFunc(200), 500},
	}
	for _, tc := range cases {
		if got := StatusCode(tc.err); got != tc.want {
			t.Errorf("%s: StatusCode = %d, want %d", tc.name, got, tc.want)
		}
	}
}

type statusCoderFunc int

func (s statusCoderFunc) Error() string   { return "coder" }
func (s statusCoderFunc) StatusCode() int { return int(s) }

func TestDefaultErrorHandlerBodies(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		body   string
	}{
		{"constructor", NotFound("widget not found"), 404, errorBodyOf(404, "widget not found")},
		{"sentinel", ErrUnauthorized, 401, errorBodyOf(401, "Unauthorized")},
		{"domain 4xx shows its message", fmt.Errorf("svc: %w", userNotFound{"7"}), 404, errorBodyOf(404, "user 7 not found")},
		{"domain 5xx hides its message", dbDown{}, 503, errorBodyOf(503, "Service Unavailable")},
		{"plain error hides its message", errors.New("pq: password authentication failed"), 500, errorBodyOf(500, "Internal Server Error")},
		{"wrapped cause is never sent", BadRequest("bad cursor").Wrap(errors.New("secret detail")), 400, errorBodyOf(400, "bad cursor")},
		{"message is escaped", BadRequest("say \"hi\"\n<b>&</b> \u2028 é"), 400,
			`{"error":{"status":400,"message":"say \"hi\"\n\u003cb\u003e\u0026\u003c/b\u003e \u2028 é"}}` + "\n"},
		{"details", Conflict("taken").WithDetail("field", "email"), 409,
			`{"error":{"status":409,"message":"taken","details":{"field":"email"}}}` + "\n"},
		{"validation without fields", &ValidationError{Err: errors.New("Key: 'User.Email' failed")}, 422, errorBodyOf(422, "validation failed")},
		{"validation with fields", &ValidationError{Err: fieldErrors{"email": "required"}}, 422,
			`{"error":{"status":422,"message":"validation failed","fields":{"email":"required"}}}` + "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c := newContext(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			defer c.release()
			DefaultErrorHandler(c, tc.err)
			if rec.Code != tc.status || rec.Body.String() != tc.body {
				t.Fatalf("got %d %q\nwant %d %q", rec.Code, rec.Body.String(), tc.status, tc.body)
			}
			if ct := rec.Header().Get(HeaderContentType); ct != jsonType {
				t.Fatalf("content-type=%q", ct)
			}
			if !json.Valid(rec.Body.Bytes()) {
				t.Fatalf("invalid JSON: %q", rec.Body.String())
			}
		})
	}
}

// The hand-written fast path must produce exactly what encoding/json would.
func TestErrorEnvelopeMatchesEncodingJSON(t *testing.T) {
	for _, msg := range []string{"plain", `q"uote`, "back\\slash", "tab\tnl\n", "<>&", "\u2028\u2029", "ctrl\x01", "é中文", "bad\xffutf8"} {
		var b strings.Builder
		if err := writeErrorEnvelope(&b, 400, msg); err != nil {
			t.Fatal(err)
		}
		want, _ := json.Marshal(errorBody{Error: errorPayload{Status: 400, Message: msg}})
		if utf8.ValidString(msg) {
			if b.String() != string(want)+"\n" {
				t.Errorf("%q:\n got %s\nwant %s", msg, b.String(), want)
			}
			continue
		}
		// Go versions differ in how encoding/json writes the replacement
		// character, raw or escaped, so compare what a client decodes.
		var got, expected errorBody
		if err := json.Unmarshal([]byte(b.String()), &got); err != nil {
			t.Fatalf("%q: invalid JSON %q: %v", msg, b.String(), err)
		}
		_ = json.Unmarshal(want, &expected)
		if !reflect.DeepEqual(got, expected) {
			t.Errorf("%q: decoded %+v, want %+v", msg, got, expected)
		}
	}
}

func TestBindingAndValidationErrorResponses(t *testing.T) {
	type query struct {
		Page  int     `query:"page"`
		Ratio float64 `query:"ratio"`
	}
	type body struct {
		Age int `json:"age"`
	}
	app := New()
	app.Get("/list", func(c *Context) error {
		var q query
		return c.Bind().Query(&q)
	})
	app.Post("/users", func(c *Context) error {
		var b body
		return c.Bind().JSON(&b)
	})
	cases := []struct {
		name, method, target, payload, want string
		status                              int
	}{
		{"query type", "GET", "/list?page=two", "", `{"error":{"status":400,"message":"invalid query parameter","fields":{"page":"must be an integer"}}}` + "\n", 400},
		{"float", "GET", "/list?ratio=x", "", `{"error":{"status":400,"message":"invalid query parameter","fields":{"ratio":"must be a number"}}}` + "\n", 400},
		{"json type", "POST", "/users", `{"age":"old"}`, `{"error":{"status":400,"message":"invalid request body","fields":{"age":"must be an integer"}}}` + "\n", 400},
		{"json syntax", "POST", "/users", `{"age":`, errorBodyOf(400, "invalid request body"), 400},
		{"empty body", "POST", "/users", ``, errorBodyOf(400, "request body is empty"), 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.payload))
			req.Header.Set(HeaderContentType, "application/json")
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)
			if rec.Code != tc.status || rec.Body.String() != tc.want {
				t.Fatalf("got %d %q\nwant %d %q", rec.Code, rec.Body.String(), tc.status, tc.want)
			}
		})
	}

	// Validator failures are 422 unless the validator chose a status itself.
	for _, tc := range []struct {
		err    error
		status int
	}{
		{errors.New("email required"), 422},
		{fieldErrors{"email": "required"}, 422},
		{BadRequest("nope"), 400},
		{userNotFound{"1"}, 404},
	} {
		v := New(Config{Validator: stubValidator{tc.err}})
		v.Post("/v", func(c *Context) error {
			var b body
			return c.Bind().JSON(&b)
		})
		req := httptest.NewRequest("POST", "/v", strings.NewReader(`{"age":1}`))
		req.Header.Set(HeaderContentType, "application/json")
		rec := httptest.NewRecorder()
		v.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Errorf("validator error %v: status %d, want %d (body %q)", tc.err, rec.Code, tc.status, rec.Body.String())
		}
	}
}

func TestTextErrorsKeepsPlainTextBodies(t *testing.T) {
	app := New(Config{ErrorHandler: TextErrors})
	app.Get("/x", func(c *Context) error { return NotFound("gone away") })
	app.Get("/boom", func(c *Context) error { return errors.New("secret") })
	for target, want := range map[string]string{"/x": "gone away", "/boom": "Internal Server Error", "/missing": "Not Found"} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
		if rec.Body.String() != want || !strings.HasPrefix(rec.Header().Get(HeaderContentType), "text/plain") {
			t.Errorf("%s: %q %q", target, rec.Body.String(), rec.Header().Get(HeaderContentType))
		}
	}
}

// Static directories and file helpers report misses through the error
// handler instead of net/http's plain-text 404, and never as 500.
func TestStaticAndFileMissesUseTheErrorHandler(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := New()
	app.Static("/assets", root)
	app.Get("/fs-missing", func(c *Context) error { return c.FileFS("nope.txt", fstest.MapFS{}) })
	app.Get("/os-missing", func(c *Context) error { return c.File(filepath.Join(root, "nope.txt")) })

	for _, tc := range []struct {
		method, target string
		status         int
		body           string
	}{
		{"GET", "/assets/app.js", 200, "asset"},
		{"GET", "/assets/missing.css", 404, errorBodyOf(404, "Not Found")},
		{"POST", "/assets/app.js", 405, errorBodyOf(405, "Method Not Allowed")},
		{"GET", "/fs-missing", 404, errorBodyOf(404, "Not Found")},
		{"GET", "/os-missing", 404, errorBodyOf(404, "Not Found")},
		{"HEAD", "/assets/missing.css", 404, ""},
	} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, nil))
		if rec.Code != tc.status || rec.Body.String() != tc.body {
			t.Errorf("%s %s: got %d %q, want %d %q", tc.method, tc.target, rec.Code, rec.Body.String(), tc.status, tc.body)
		}
		if tc.status == 405 && rec.Header().Get(HeaderAllow) != "GET, HEAD" {
			t.Errorf("405 Allow = %q", rec.Header().Get(HeaderAllow))
		}
	}
}

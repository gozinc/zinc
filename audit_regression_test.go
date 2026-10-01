// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware/bodydump"
	"github.com/0mjs/zinc/middleware/compress"
	"github.com/0mjs/zinc/middleware/cors"
	"github.com/0mjs/zinc/middleware/csrf"
	"github.com/0mjs/zinc/middleware/rewrite"
)

// These tests come from the 0.7 framework audit's probes (findings F03 to
// F11 and the API observations). Each asserts the behavior Zinc should have.
// Until the phase that fixes it lands, pending skips it with that phase and
// finding, so `go test -v -run Audit` lists what's still open. The phase
// removes its pending call.
//
// The OpenAPI findings (F01, F02, F12, F13) are scenarios in the openapitest
// audit, where the baseline tracks them.

// pending skips a test whose fix is planned for phase. With
// ZINC_AUDIT_OPEN=1 it runs instead, to show the test still fails.
func pending(t *testing.T, phase, finding string) {
	t.Helper()
	if os.Getenv("ZINC_AUDIT_OPEN") != "1" {
		t.Skipf("open: %s fixes %s", phase, finding)
	}
}

func serve(app *zinc.App, method, target, body string, header ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}

func mustPanicAudit(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected a registration panic")
		}
	}()
	f()
}

// F07: a handler's explicit status wins over the route's, including 200.
func TestAuditExplicitStatusWins(t *testing.T) {
	app := zinc.New()
	app.Post("/", zinc.Typed(func(c *zinc.Context, _ struct{}) (string, error) {
		c.Status(http.StatusOK)
		return "ok", nil
	})).Status(http.StatusCreated)
	if w := serve(app, "POST", "/", ""); w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
}

// D7: a plain handler's route status is its runtime default too.
func TestAuditPlainRouteStatus(t *testing.T) {
	pending(t, "P18", "D7")
	app := zinc.New()
	app.Post("/", func(c *zinc.Context) error { return c.JSON("ok") }).Status(http.StatusCreated)
	if w := serve(app, "POST", "/", ""); w.Code != http.StatusCreated {
		t.Fatalf("status %d, want 201", w.Code)
	}
}

// F03: a field tagged only for the URL or headers can't be set by the body.
func TestAuditSourceOwnership(t *testing.T) {
	pending(t, "P17", "F03")
	type input struct {
		Role   string `header:"X-Role"`
		DryRun bool   `query:"dry_run"`
		Name   string `json:"name"`
	}
	app := zinc.New()
	app.Post("/", zinc.Typed(func(_ *zinc.Context, in input) (input, error) { return in, nil }))
	w := serve(app, "POST", "/", `{"Role":"admin","DryRun":true,"name":"Alice"}`, "Content-Type", "application/json")
	if body := w.Body.String(); !strings.Contains(body, `"Role":""`) || !strings.Contains(body, `"DryRun":false`) || !strings.Contains(body, `"name":"Alice"`) {
		t.Fatalf("the body filled parameter fields: %s", body)
	}
}

// F08: Bind().All binds what a typed handler binds, headers and cookies
// included.
func TestAuditAllMatchesTyped(t *testing.T) {
	pending(t, "P17", "F08")
	type input struct {
		Token   string `header:"X-Token" json:"-"`
		Session string `cookie:"session" json:"-"`
	}
	app := zinc.New()
	app.Post("/plain", func(c *zinc.Context) error {
		var in input
		if err := c.Bind().All(&in); err != nil {
			return err
		}
		return c.String(in.Token + "/" + in.Session)
	})
	app.Post("/typed", zinc.Typed(func(_ *zinc.Context, in input) (string, error) { return in.Token + "/" + in.Session, nil }))
	plain := serve(app, "POST", "/plain", "", "X-Token", "secret", "Cookie", "session=abc").Body.String()
	typed := strings.TrimSpace(serve(app, "POST", "/typed", "", "X-Token", "secret", "Cookie", "session=abc").Body.String())
	if plain != "secret/abc" || typed != `"secret/abc"` {
		t.Fatalf("plain %q, typed %q", plain, typed)
	}
}

// F06: tagged fields of an embedded struct bind, defaults included.
func TestAuditEmbeddedBinding(t *testing.T) {
	type pagination struct {
		Limit int `query:"limit" default:"20"`
	}
	type input struct {
		pagination
		Name string `query:"name"`
	}
	app := zinc.New()
	app.Get("/", zinc.Typed(func(_ *zinc.Context, in input) (int, error) { return in.Limit, nil }))
	if got := strings.TrimSpace(serve(app, "GET", "/?limit=7", "").Body.String()); got != "7" {
		t.Fatalf("limit %s, want 7", got)
	}
	if got := strings.TrimSpace(serve(app, "GET", "/", "").Body.String()); got != "20" {
		t.Fatalf("default limit %s, want 20", got)
	}
}

// A parameter field of a type binding can't fill fails at registration.
func TestAuditUnsupportedFieldRejected(t *testing.T) {
	type input struct {
		Options map[string]string `query:"options"`
	}
	mustPanicAudit(t, func() {
		zinc.New().Get("/", zinc.Typed(func(_ *zinc.Context, in input) (input, error) { return in, nil }))
	})
}

// F09: an oversized form reports the size limit, not a missing field.
func TestAuditFormKeepsItsError(t *testing.T) {
	app := zinc.New(zinc.Config{BodyLimit: 4})
	app.Post("/", func(c *zinc.Context) error {
		v, err := zinc.Form[string](c, "name")
		if err != nil {
			return err
		}
		return c.JSON(v)
	})
	if w := serve(app, "POST", "/", "name=abcdef", "Content-Type", "application/x-www-form-urlencoded"); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413: %s", w.Code, w.Body)
	}
}

// F04: CSRF's form reader honors Config.BodyLimit.
func TestAuditCSRFFormBodyLimit(t *testing.T) {
	app := zinc.New(zinc.Config{BodyLimit: 4})
	app.Use(csrf.New(csrf.Config{
		Readers:  []csrf.Reader{csrf.FromForm("token")},
		Generate: func(*zinc.Context) (string, error) { return strings.Repeat("a", 32), nil },
	}))
	app.Get("/", func(c *zinc.Context) error { return c.NoContent() })
	app.Post("/", func(c *zinc.Context) error { return c.NoContent() })
	cookie := serve(app, "GET", "/", "").Result().Cookies()[0]
	w := serve(app, "POST", "/", "token="+cookie.Value+"&payload="+strings.Repeat("x", 100),
		"Cookie", cookie.String(), "Content-Type", "application/x-www-form-urlencoded")
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", w.Code)
	}
}

// F10: bodydump records the response the client got.
func TestAuditBodydumpSeesTheResponse(t *testing.T) {
	pending(t, "P15", "F10")
	var seen bodydump.Snapshot
	app := zinc.New()
	app.Use(bodydump.New(bodydump.Config{Observe: func(_ *zinc.Context, s bodydump.Snapshot) { seen = s }}))
	app.Get("/", func(c *zinc.Context) error { _, err := zinc.Query[int](c, "id"); return err })
	w := serve(app, "GET", "/?id=abc", "")
	if seen.Status != w.Code || string(seen.ResponseBody) != w.Body.String() {
		t.Fatalf("snapshot %d %q, response %d %q", seen.Status, seen.ResponseBody, w.Code, w.Body)
	}
}

// F11: wrapping rewrite in Skip keeps the check that it can't run on a
// group.
func TestAuditSkipKeepsPreroutingMark(t *testing.T) {
	pending(t, "P15", "F11")
	mw := zinc.Skip(func(*zinc.Context) bool { return false }, rewrite.New(rewrite.Config{Rules: map[string]string{"/api/old": "/api/new"}}))
	mustPanicAudit(t, func() { zinc.New().Group("/api", mw) })
}

// F05: CORS on a group answers its routes' preflight requests.
func TestAuditGroupCORSPreflight(t *testing.T) {
	pending(t, "P19", "F05")
	app := zinc.New()
	app.Group("/api", cors.New()).Post("/pets", func(c *zinc.Context) error { return c.JSON("ok") })
	w := serve(app, "OPTIONS", "/api/pets", "", "Origin", "https://example.com", "Access-Control-Request-Method", "POST")
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatalf("no Access-Control-Allow-Origin on the preflight: %d %v", w.Code, w.Header())
	}
}

// D2: the built-in spec endpoint shows in route inspection.
func TestAuditBuiltinsInRoutes(t *testing.T) {
	pending(t, "P20", "D2")
	app := zinc.New()
	if _, ok := app.FindRoute("GET", "/openapi.json"); !ok {
		t.Fatal("FindRoute doesn't find /openapi.json")
	}
}

// A GET body is either bound and documented, or neither.
func TestAuditGETBodyPolicy(t *testing.T) {
	pending(t, "P17", "GET body policy")
	type input struct {
		Name string `json:"name"`
	}
	app := zinc.New()
	app.Get("/", zinc.Typed(func(_ *zinc.Context, in input) (input, error) { return in, nil }))
	bound := strings.Contains(serve(app, "GET", "/", `{"name":"x"}`, "Content-Type", "application/json").Body.String(), `"x"`)
	spec, _ := app.OpenAPISpec(zinc.OpenAPIConfig{})
	if documented := strings.Contains(string(spec), `"requestBody"`); bound != documented {
		t.Fatalf("GET body bound %v, documented %v", bound, documented)
	}
}

// The compressed writer doesn't claim http.Hijacker when the underlying
// writer can't hijack. This one already holds.
func TestAuditCompressDoesNotClaimHijacker(t *testing.T) {
	var hijacker bool
	app := zinc.New()
	app.Use(compress.New())
	app.Get("/", func(c *zinc.Context) error { _, hijacker = c.Writer().(http.Hijacker); return c.NoContent() })
	serve(app, "GET", "/", "", "Accept-Encoding", "gzip")
	if hijacker {
		t.Fatal("the compressed writer claims http.Hijacker over a recorder")
	}
}

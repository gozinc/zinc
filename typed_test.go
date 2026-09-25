// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type createMember struct {
	OrgID   string `path:"org"`
	DryRun  bool   `query:"dry_run"`
	TraceID string `header:"X-Trace-ID"`
	Email   string `json:"email"`
}

type member struct {
	Org    string `json:"org"`
	Email  string `json:"email"`
	DryRun bool   `json:"dry_run"`
	Trace  string `json:"trace"`
}

type emailRequired struct{}

func (emailRequired) Validate(v any) error {
	if in, ok := v.(*createMember); ok && in.Email == "" {
		return fieldErrors{"email": "required"}
	}
	return nil
}

func typedApp() *App {
	app := New(Config{Validator: emailRequired{}})
	api := app.Group("/api")
	api.Post("/orgs/{org}/members", Typed(func(c *Context, in createMember) (member, error) {
		return member{Org: in.OrgID, Email: in.Email, DryRun: in.DryRun, Trace: in.TraceID}, nil
	})).Status(http.StatusCreated).Name("members.create")
	app.Get("/members/{id}", Typed(func(c *Context, in struct {
		ID int `path:"id"`
	}) (member, error) {
		if in.ID == 404 {
			return member{}, NotFound("member not found")
		}
		return member{Email: "ada@example.com"}, nil
	}))
	app.Delete("/members/{id}", Typed(func(c *Context, in struct{}) (NoContent, error) {
		return NoContent{}, nil
	}))
	app.Post("/jobs", Typed(func(c *Context, in struct{}) (NoContent, error) {
		return NoContent{}, nil
	})).Status(http.StatusAccepted)
	app.Put("/members/{id}", Typed(func(c *Context, in struct{}) (member, error) {
		c.Status(http.StatusAccepted)
		return member{Email: "queued"}, nil
	})).Status(http.StatusCreated)
	app.Get("/legacy", Typed(func(c *Context, in struct{}) (member, error) {
		return member{}, c.Redirect("/members/1")
	}))
	return app
}

func TestTypedHandlers(t *testing.T) {
	app := typedApp()
	cases := []struct {
		name, method, target, body string
		header                     map[string]string
		status                     int
		want                       string
	}{
		{"binds every source and declares 201", "POST", "/api/orgs/acme/members?dry_run=true",
			`{"email":"ada@example.com"}`, map[string]string{"X-Trace-ID": "t-1"}, 201,
			`{"org":"acme","email":"ada@example.com","dry_run":true,"trace":"t-1"}` + "\n"},
		{"default 200", "GET", "/members/7", "", nil, 200, `{"org":"","email":"ada@example.com","dry_run":false,"trace":""}` + "\n"},
		{"returned error", "GET", "/members/404", "", nil, 404, `{"error":{"status":404,"message":"member not found"}}` + "\n"},
		{"bad input is 400", "GET", "/members/abc", "", nil, 400,
			`{"error":{"status":400,"message":"invalid path parameter","fields":{"id":"must be an integer"}}}` + "\n"},
		{"validation is 422", "POST", "/api/orgs/acme/members", `{"email":""}`, nil, 422,
			`{"error":{"status":422,"message":"validation failed","fields":{"email":"required"}}}` + "\n"},
		{"NoContent is 204", "DELETE", "/members/7", "", nil, 204, ""},
		{"declared status with NoContent", "POST", "/jobs", "", nil, 202, ""},
		{"handler status wins", "PUT", "/members/7", "", nil, 202, `{"org":"","email":"queued","dry_run":false,"trace":""}` + "\n"},
		{"handler writes its own response", "GET", "/legacy", "", nil, 302, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set(HeaderContentType, "application/json")
			}
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, req)
			if rec.Code != tc.status || rec.Body.String() != tc.want {
				t.Fatalf("got %d %q\nwant %d %q", rec.Code, rec.Body.String(), tc.status, tc.want)
			}
		})
	}

	if info, ok := app.RouteByName("members.create"); !ok || info.Status != http.StatusCreated {
		t.Fatalf("RouteInfo.Status = %+v %v", info, ok)
	}
}

func TestTypedRejectsMisuse(t *testing.T) {
	for name, fn := range map[string]func(){
		"non-struct input": func() { Typed(func(c *Context, id int) (member, error) { return member{}, nil }) },
		"nil function":     func() { Typed[struct{}, member](nil) },
		"non-2xx status": func() {
			New().Get("/x", Typed(func(c *Context, in struct{}) (member, error) { return member{}, nil })).Status(302)
		},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			fn()
		}()
	}
}

// A typed handler allocates no more than the hand-written equivalent.
func TestTypedMatchesHandwrittenAllocations(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are unreliable under -race")
	}
	type in struct {
		ID    int    `path:"id"`
		Email string `json:"email"`
	}
	typed := New()
	typed.Post("/u/{id}", Typed(func(c *Context, v in) (member, error) { return member{Email: v.Email}, nil }))
	manual := New()
	manual.Post("/u/{id}", func(c *Context) error {
		var v in
		if err := c.Bind().All(&v); err != nil {
			return err
		}
		return c.JSON(member{Email: v.Email})
	})
	measure := func(app *App) float64 {
		return testing.AllocsPerRun(200, func() {
			req := httptest.NewRequest("POST", "/u/7", strings.NewReader(`{"email":"a@b.c"}`))
			req.Header.Set(HeaderContentType, "application/json")
			app.ServeHTTP(&discardWriter{header: http.Header{}}, req)
		})
	}
	if tAllocs, mAllocs := measure(typed), measure(manual); tAllocs > mAllocs {
		t.Fatalf("Typed allocates %.1f, hand-written %.1f", tAllocs, mAllocs)
	}

}

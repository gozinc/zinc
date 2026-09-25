// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"errors"
	"net/http"
	"strconv"
	"testing"

	. "github.com/0mjs/zinc"
)

// The API04 benchmarks track the Zinc operations that the 0.4 API work
// changes. They are Zinc-only and outside the head-to-head suite. Each is named
// by operation rather than by API: when a phase replaces an API, the body is
// rewritten with the new API under the same name, and zincbench compare
// measures it against the record taken with the old one. The workload
// (request, route, response semantics) must stay the same across rewrites.

type api04User struct {
	ID   int
	Name string
}

type api04ContextKey int

const api04UserKey api04ContextKey = iota

var errAPI04Validation = errors.New("name is required")

type api04Validator struct{}

func (api04Validator) Validate(any) error { return errAPI04Validation }

// api04Workloads is shared by the benchmarks and TestAPI04Workloads, which
// proves each one takes the intended path before it is timed.
var api04Workloads = []struct {
	name   string
	build  func() http.Handler
	req    func() preparedBenchmarkRequest
	status int
}{
	{
		name: "ErrorSentinel",
		build: func() http.Handler {
			app := New()
			app.Get("/users/{id}", func(c *Context) error { return ErrNotFound })
			return app
		},
		req: func() preparedBenchmarkRequest {
			return newPreparedBenchmarkRequest(http.MethodGet, "/users/42", nil, nil)
		},
		status: http.StatusNotFound,
	},
	{
		name: "ErrorWithMessage",
		build: func() http.Handler {
			app := New()
			app.Get("/users/{id}", func(c *Context) error {
				return BadRequest("id must be an integer")
			})
			return app
		},
		req: func() preparedBenchmarkRequest {
			return newPreparedBenchmarkRequest(http.MethodGet, "/users/abc", nil, nil)
		},
		status: http.StatusBadRequest,
	},
	{
		name: "ErrorBindInvalidJSON",
		build: func() http.Handler {
			app := New()
			app.Post("/payload", func(c *Context) error {
				var input benchmarkValidationInput
				if err := c.Bind().JSON(&input); err != nil {
					return err
				}
				return c.NoContent()
			})
			return app
		},
		req: func() preparedBenchmarkRequest {
			return newPreparedBenchmarkRequest(http.MethodPost, "/payload", benchmarkInvalidJSONBody,
				http.Header{"Content-Type": {"application/json"}})
		},
		status: http.StatusBadRequest,
	},
	{
		// 0.3 answered a validator failure with 500; 0.4 answers 422.
		name: "ErrorValidation",
		build: func() http.Handler {
			cfg := Config{}
			cfg.Validator = api04Validator{}
			app := New(cfg)
			app.Post("/validate", func(c *Context) error {
				var input benchmarkValidationInput
				if err := c.Bind().JSON(&input); err != nil {
					return err
				}
				return c.NoContent()
			})
			return app
		},
		req: func() preparedBenchmarkRequest {
			return newPreparedBenchmarkRequest(http.MethodPost, "/validate", benchmarkValidationFailureBody,
				http.Header{"Content-Type": {"application/json"}})
		},
		status: http.StatusUnprocessableEntity,
	},
	{
		name: "ParamInt",
		build: func() http.Handler {
			app := New()
			app.Get("/users/{id}", func(c *Context) error {
				id, err := Param[int](c, "id")
				if err != nil {
					return err
				}
				benchmarkSinkInt = id
				return c.NoContent()
			})
			return app
		},
		req: func() preparedBenchmarkRequest {
			return newPreparedBenchmarkRequest(http.MethodGet, "/users/42", nil, nil)
		},
		status: http.StatusNoContent,
	},
	{
		name: "QueryIntDefault",
		build: func() http.Handler {
			app := New()
			app.Get("/users", func(c *Context) error {
				page := QueryOr(c, "page", 1)
				limit := QueryOr(c, "limit", 20)
				benchmarkSinkInt = page + limit
				return c.NoContent()
			})
			return app
		},
		req: func() preparedBenchmarkRequest {
			return newPreparedBenchmarkRequest(http.MethodGet, "/users?page=3", nil, nil)
		},
		status: http.StatusNoContent,
	},
	{
		name: "StoreValue",
		build: func() http.Handler {
			user := &api04User{ID: 42, Name: "Ada"}
			app := New()
			app.Use(func(c *Context) error {
				c.Set(api04UserKey, user)
				return c.Next()
			})
			app.Get("/me", func(c *Context) error {
				u, _ := Value[*api04User](c, api04UserKey)
				benchmarkSinkInt = u.ID
				return c.NoContent()
			})
			return app
		},
		req:    func() preparedBenchmarkRequest { return newPreparedBenchmarkRequest(http.MethodGet, "/me", nil, nil) },
		status: http.StatusNoContent,
	},
	{
		name: "HeaderRead",
		build: func() http.Handler {
			app := New()
			app.Get("/secure", func(c *Context) error {
				benchmarkSinkBool = c.Header("Authorization") != ""
				return c.NoContent()
			})
			return app
		},
		req: func() preparedBenchmarkRequest {
			return newPreparedBenchmarkRequest(http.MethodGet, "/secure", nil, http.Header{"Authorization": {"Bearer token"}})
		},
		status: http.StatusNoContent,
	},
	{
		name: "Redirect",
		build: func() http.Handler {
			app := New()
			app.Get("/old", func(c *Context) error { return c.Redirect("/login") })
			return app
		},
		req:    func() preparedBenchmarkRequest { return newPreparedBenchmarkRequest(http.MethodGet, "/old", nil, nil) },
		status: http.StatusFound,
	},
	{
		name: "PreencodedJSON",
		build: func() http.Handler {
			body := []byte(`{"id":42,"name":"Ada"}`)
			app := New()
			app.Get("/user", func(c *Context) error { return c.Data(MIMEJSON, body) })
			return app
		},
		req:    func() preparedBenchmarkRequest { return newPreparedBenchmarkRequest(http.MethodGet, "/user", nil, nil) },
		status: http.StatusOK,
	},
}

// api04CreateInput and api04CreateOutput are the typed-handler workload: a
// create request bound from path, query, header, and a JSON body.
type api04CreateInput struct {
	OrgID   string `path:"org"`
	DryRun  bool   `query:"dry_run"`
	TraceID string `header:"X-Trace-ID"`
	Email   string `json:"email"`
}

type api04CreateOutput struct {
	Org   string `json:"org"`
	Email string `json:"email"`
}

func api04CreateRequest() preparedBenchmarkRequest {
	return newPreparedBenchmarkRequest(http.MethodPost, "/orgs/acme/users?dry_run=true", []byte(`{"email":"ada@example.com"}`),
		http.Header{"Content-Type": {"application/json"}, "X-Trace-Id": {"t-1"}})
}

func init() {
	api04Workloads = append(api04Workloads,
		struct {
			name   string
			build  func() http.Handler
			req    func() preparedBenchmarkRequest
			status int
		}{
			name: "TypedCreate",
			build: func() http.Handler {
				app := New()
				app.Post("/orgs/{org}/users", Typed(func(c *Context, in api04CreateInput) (api04CreateOutput, error) {
					return api04CreateOutput{Org: in.OrgID, Email: in.Email}, nil
				})).Status(http.StatusCreated)
				return app
			},
			req:    api04CreateRequest,
			status: http.StatusCreated,
		},
		struct {
			name   string
			build  func() http.Handler
			req    func() preparedBenchmarkRequest
			status int
		}{
			// The same contract written by hand, for comparison with TypedCreate.
			name: "HandwrittenCreate",
			build: func() http.Handler {
				app := New()
				app.Post("/orgs/{org}/users", func(c *Context) error {
					var in api04CreateInput
					if err := c.Bind().Header(&in); err != nil {
						return err
					}
					if err := c.Bind().All(&in); err != nil {
						return err
					}
					return c.Status(http.StatusCreated).JSON(api04CreateOutput{Org: in.OrgID, Email: in.Email})
				})
				return app
			},
			req:    api04CreateRequest,
			status: http.StatusCreated,
		},
	)
}

func runAPI04Workload(b *testing.B, name string) {
	for _, w := range api04Workloads {
		if w.name != name {
			continue
		}
		handler := w.build()
		req := w.req()
		rw := newDiscardResponseWriter()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rw.reset()
			handler.ServeHTTP(rw, req.reset())
		}
		benchmarkSinkInt = rw.status + rw.bytes
		return
	}
	b.Fatalf("unknown API04 workload %q", name)
}

func BenchmarkAPI04ErrorSentinel(b *testing.B)        { runAPI04Workload(b, "ErrorSentinel") }
func BenchmarkAPI04ErrorWithMessage(b *testing.B)     { runAPI04Workload(b, "ErrorWithMessage") }
func BenchmarkAPI04ErrorBindInvalidJSON(b *testing.B) { runAPI04Workload(b, "ErrorBindInvalidJSON") }
func BenchmarkAPI04ErrorValidation(b *testing.B)      { runAPI04Workload(b, "ErrorValidation") }
func BenchmarkAPI04ParamInt(b *testing.B)             { runAPI04Workload(b, "ParamInt") }
func BenchmarkAPI04QueryIntDefault(b *testing.B)      { runAPI04Workload(b, "QueryIntDefault") }
func BenchmarkAPI04StoreValue(b *testing.B)           { runAPI04Workload(b, "StoreValue") }
func BenchmarkAPI04HeaderRead(b *testing.B)           { runAPI04Workload(b, "HeaderRead") }
func BenchmarkAPI04Redirect(b *testing.B)             { runAPI04Workload(b, "Redirect") }
func BenchmarkAPI04PreencodedJSON(b *testing.B)       { runAPI04Workload(b, "PreencodedJSON") }
func BenchmarkAPI04TypedCreate(b *testing.B)          { runAPI04Workload(b, "TypedCreate") }
func BenchmarkAPI04HandwrittenCreate(b *testing.B)    { runAPI04Workload(b, "HandwrittenCreate") }

// BenchmarkAPI04RouteRegistration registers 1,000 parameter routes. P5 makes
// the method helpers return a Route; this must stay allocation-neutral.
func BenchmarkAPI04RouteRegistration(b *testing.B) {
	paths := make([]string, 1000)
	for i := range paths {
		paths[i] = "/resource" + strconv.Itoa(i) + "/{id}"
	}
	handler := func(c *Context) error { return nil }
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app := New()
		for _, path := range paths {
			app.Get(path, handler)
		}
	}
}

func TestAPI04Workloads(t *testing.T) {
	for _, w := range api04Workloads {
		t.Run(w.name, func(t *testing.T) {
			handler := w.build()
			req := w.req()
			rw := newDiscardResponseWriter()
			handler.ServeHTTP(rw, req.reset())
			if rw.status != w.status {
				t.Fatalf("status=%d, want %d", rw.status, w.status)
			}
		})
	}
}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	. "github.com/0mjs/zinc"
)

var zincDiagnosticBindJSONBody = []byte(`{"name":"alice","roles":["admin","editor"]}`)

type zincDiagnosticBindQueryInput struct {
	Page    int    `query:"page"`
	Search  string `query:"search"`
	Enabled bool   `query:"enabled"`
}

type zincDiagnosticBindAPIInput struct {
	TeamID  int      `path:"teamID"`
	UserID  int      `path:"userID"`
	Verbose bool     `query:"verbose"`
	Limit   int      `query:"limit"`
	Name    string   `json:"name"`
	Roles   []string `json:"roles"`
}

type zincDiagnosticBindFormInput struct {
	Name  string   `form:"name"`
	Count int      `form:"count"`
	Tags  []string `form:"tags"`
}

func provePreparedResponse(t testing.TB, handler http.Handler, request preparedBenchmarkRequest, wantStatus int, wantBody string) {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, request.reset())

	if rec.Code != wantStatus {
		t.Fatalf("status=%d want=%d", rec.Code, wantStatus)
	}
	if rec.Body.String() != wantBody {
		t.Fatalf("body=%q want=%q", rec.Body.String(), wantBody)
	}
}

func runPreparedZincBenchmark(b *testing.B, handler http.Handler, request preparedBenchmarkRequest) {
	rw := newDiscardResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rw.reset()
		handler.ServeHTTP(rw, request.reset())
	}

	benchmarkSinkInt += rw.status + rw.bytes
}

func buildZincBindQueryOnlyHandler() http.Handler {
	app := New()
	app.Get("/search", func(c *Context) error {
		var input zincDiagnosticBindQueryInput
		if err := c.Bind().Query(&input); err != nil {
			return err
		}
		benchmarkSinkInt = input.Page
		benchmarkSinkString = input.Search
		benchmarkSinkBool = input.Enabled
		return c.NoContent()
	})
	return app
}

func buildZincBindPathQueryJSONHandler() http.Handler {
	app := New()
	app.Post("/teams/{teamID}/users/{userID}", func(c *Context) error {
		var input zincDiagnosticBindAPIInput
		if err := c.Bind().All(&input); err != nil {
			return err
		}
		benchmarkSinkInt = input.TeamID + input.UserID + input.Limit + len(input.Roles)
		benchmarkSinkString = input.Name
		benchmarkSinkBool = input.Verbose
		return c.NoContent()
	})
	return app
}

func buildZincBindJSONCachedBodyHandler() http.Handler {
	app := New()
	app.Post("/payload", func(c *Context) error {
		var proof zincDiagnosticBindAPIInput
		if err := c.Bind().JSON(&proof); err != nil {
			return err
		}

		var input zincDiagnosticBindAPIInput
		if err := c.Bind().JSON(&input); err != nil {
			return err
		}

		benchmarkSinkInt = len(input.Roles)
		benchmarkSinkString = input.Name
		benchmarkSinkBool = proof.Name == input.Name && len(proof.Roles) == len(input.Roles)
		return c.NoContent()
	})
	return app
}

func buildZincBindFormHandler() http.Handler {
	app := New()
	app.Post("/submit", func(c *Context) error {
		var input zincDiagnosticBindFormInput
		if err := c.Bind().Form(&input); err != nil {
			return err
		}
		benchmarkSinkInt = input.Count + len(input.Tags)
		benchmarkSinkString = input.Name
		benchmarkSinkBool = len(input.Tags) == 2
		return c.NoContent()
	})
	return app
}

func BenchmarkZincBindQueryOnly(b *testing.B) {
	handler := buildZincBindQueryOnlyHandler()
	request := newPreparedBenchmarkRequest(MethodGet, "/search?page=2&search=alpha&enabled=true", nil, nil)

	benchmarkSinkInt = 0
	benchmarkSinkString = ""
	benchmarkSinkBool = false
	provePreparedResponse(b, handler, request, http.StatusNoContent, "")
	if benchmarkSinkInt != 2 || benchmarkSinkString != "alpha" || !benchmarkSinkBool {
		b.Fatalf("unexpected bind proof: page=%d search=%q enabled=%v", benchmarkSinkInt, benchmarkSinkString, benchmarkSinkBool)
	}

	runPreparedZincBenchmark(b, handler, request)
}

func BenchmarkZincBindPathQueryJSON(b *testing.B) {
	handler := buildZincBindPathQueryJSONHandler()
	request := newPreparedBenchmarkRequest(MethodPost, "/teams/42/users/7?verbose=true&limit=25", zincDiagnosticBindJSONBody, benchmarkAPIBodyHeaders())

	benchmarkSinkInt = 0
	benchmarkSinkString = ""
	benchmarkSinkBool = false
	provePreparedResponse(b, handler, request, http.StatusNoContent, "")
	if benchmarkSinkInt != 42+7+25+2 || benchmarkSinkString != "alice" || !benchmarkSinkBool {
		b.Fatalf("unexpected bind proof: sink=%d name=%q verbose=%v", benchmarkSinkInt, benchmarkSinkString, benchmarkSinkBool)
	}

	runPreparedZincBenchmark(b, handler, request)
}

func BenchmarkZincBindJSONCachedBody(b *testing.B) {
	handler := buildZincBindJSONCachedBodyHandler()
	request := newPreparedBenchmarkRequest(MethodPost, "/payload", zincDiagnosticBindJSONBody, benchmarkAPIBodyHeaders())

	benchmarkSinkInt = 0
	benchmarkSinkString = ""
	benchmarkSinkBool = false
	provePreparedResponse(b, handler, request, http.StatusNoContent, "")
	if benchmarkSinkInt != 2 || benchmarkSinkString != "alice" || !benchmarkSinkBool {
		b.Fatalf("unexpected cached bind proof: sink=%d name=%q cached=%v", benchmarkSinkInt, benchmarkSinkString, benchmarkSinkBool)
	}

	runPreparedZincBenchmark(b, handler, request)
}

func BenchmarkZincBindForm(b *testing.B) {
	formBody := []byte(url.Values{
		"name":  {"mia"},
		"count": {"25"},
		"tags":  {"alpha", "beta"},
	}.Encode())
	handler := buildZincBindFormHandler()
	request := newPreparedBenchmarkRequest(MethodPost, "/submit", formBody, http.Header{
		HeaderContentType: {"application/x-www-form-urlencoded"},
	})

	benchmarkSinkInt = 0
	benchmarkSinkString = ""
	benchmarkSinkBool = false
	provePreparedResponse(b, handler, request, http.StatusNoContent, "")
	if benchmarkSinkInt != 25+2 || benchmarkSinkString != "mia" || !benchmarkSinkBool {
		b.Fatalf("unexpected form bind proof: sink=%d name=%q ok=%v", benchmarkSinkInt, benchmarkSinkString, benchmarkSinkBool)
	}

	runPreparedZincBenchmark(b, handler, request)
}

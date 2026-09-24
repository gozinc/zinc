package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/0mjs/zinc"
)

var zincParams = regexp.MustCompile(`:([^/]+)`)

func zincPattern(path string) string {
	return zincParams.ReplaceAllString(path, `{$1}`)
}

func zincHandler(*zinc.Context) error { return nil }

func zincHandlerWrite(c *zinc.Context) error {
	_, err := io.WriteString(c.Writer(), c.Param("name"))
	return err
}

func zincHandlerWriteString(c *zinc.Context) error {
	return c.String(c.Param("name"))
}

func zincHandlerTest(c *zinc.Context) error {
	return c.String(c.Request().RequestURI)
}

func loadZinc(routes []route) http.Handler {
	handler := zincHandler
	if loadTestHandler {
		handler = zincHandlerTest
	}
	app := zinc.New()
	for _, r := range routes {
		app.Add(r.method, zincPattern(r.path), handler)
	}
	return app
}

func loadZincSingle(method, path string, handler zinc.HandlerFunc) http.Handler {
	app := zinc.New()
	app.Add(method, zincPattern(path), handler)
	return app
}

func TestZincParamWrite(t *testing.T) {
	for name, handler := range map[string]zinc.HandlerFunc{
		"direct": zincHandlerWrite,
		"string": zincHandlerWriteString,
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			loadZincSingle("GET", "/user/:name", handler).ServeHTTP(w, mustZincRequest("/user/gordon"))
			if w.Code != http.StatusOK || w.Body.String() != "gordon" {
				t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
			}
		})
	}
}

var (
	githubZinc http.Handler
	gplusZinc  http.Handler
	parseZinc  http.Handler
	staticZinc http.Handler
)

func init() {
	loadZincFixture("GitHub", &githubZinc, githubAPI)
	loadZincFixture("GPlus", &gplusZinc, gplusAPI)
	loadZincFixture("Parse", &parseZinc, parseAPI)
	loadZincFixture("Static", &staticZinc, staticRoutes)
}

func loadZincFixture(name string, target *http.Handler, routes []route) {
	println("#Zinc", name, "routing-structure memory:")
	calcMem("Zinc", func() { *target = loadZinc(routes) })
	if *target == nil {
		*target = loadZinc(routes)
	}
}

func BenchmarkZinc_Param(b *testing.B) {
	benchRequest(b, loadZincSingle("GET", "/user/:name", zincHandler), mustZincRequest("/user/gordon"))
}

func BenchmarkZinc_Param5(b *testing.B) {
	benchRequest(b, loadZincSingle("GET", fiveColon, zincHandler), mustZincRequest(fiveRoute))
}

func BenchmarkZinc_Param20(b *testing.B) {
	benchRequest(b, loadZincSingle("GET", twentyColon, zincHandler), mustZincRequest(twentyRoute))
}

func BenchmarkZinc_ParamWrite(b *testing.B) {
	benchRequest(b, loadZincSingle("GET", "/user/:name", zincHandlerWrite), mustZincRequest("/user/gordon"))
}

func BenchmarkZinc_ParamWriteString(b *testing.B) {
	benchRequest(b, loadZincSingle("GET", "/user/:name", zincHandlerWriteString), mustZincRequest("/user/gordon"))
}

func BenchmarkZinc_GithubStatic(b *testing.B) {
	benchRequest(b, githubZinc, mustZincRequest("/user/repos"))
}

func BenchmarkZinc_GithubParam(b *testing.B) {
	benchRequest(b, githubZinc, mustZincRequest("/repos/julienschmidt/httprouter/stargazers"))
}

func BenchmarkZinc_GithubAll(b *testing.B) { benchRoutes(b, githubZinc, githubAPI) }

func BenchmarkZinc_GPlusStatic(b *testing.B) {
	benchRequest(b, gplusZinc, mustZincRequest("/people"))
}

func BenchmarkZinc_GPlusParam(b *testing.B) {
	benchRequest(b, gplusZinc, mustZincRequest("/people/118051310819094153327"))
}

func BenchmarkZinc_GPlus2Params(b *testing.B) {
	benchRequest(b, gplusZinc, mustZincRequest("/people/118051310819094153327/activities/123456789"))
}

func BenchmarkZinc_GPlusAll(b *testing.B) { benchRoutes(b, gplusZinc, gplusAPI) }

func BenchmarkZinc_ParseStatic(b *testing.B) {
	benchRequest(b, parseZinc, mustZincRequest("/1/users"))
}

func BenchmarkZinc_ParseParam(b *testing.B) {
	benchRequest(b, parseZinc, mustZincRequest("/1/classes/go"))
}

func BenchmarkZinc_Parse2Params(b *testing.B) {
	benchRequest(b, parseZinc, mustZincRequest("/1/classes/go/123456789"))
}

func BenchmarkZinc_ParseAll(b *testing.B) { benchRoutes(b, parseZinc, parseAPI) }

func BenchmarkZinc_StaticAll(b *testing.B) { benchRoutes(b, staticZinc, staticRoutes) }

func mustZincRequest(path string) *http.Request {
	r, err := http.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		panic(err)
	}
	return r
}

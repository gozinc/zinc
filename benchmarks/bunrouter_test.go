// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"net/http"
	"strings"

	"github.com/uptrace/bunrouter"
)

// BunRouter is a bare net/http router. It runs only the routing scenarios and
// is scored in the router table, next to Chi.

// bunApp is a BunRouter with the suite's normalised miss responses. BunRouter
// never sets Allow on a 405 and keeps a route's methods private, so, as a
// BunRouter user would, bunApp records each pattern's methods at
// registration and sets Allow from them.
type bunApp struct {
	*bunrouter.Router
	allow map[string]string
}

func newBunApp() *bunApp {
	a := &bunApp{allow: map[string]string{}}
	a.Router = bunrouter.New(
		bunrouter.WithNotFoundHandler(func(w http.ResponseWriter, _ bunrouter.Request) error {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, err := w.Write([]byte(benchmarkNotFoundResponse))
			return err
		}),
		bunrouter.WithMethodNotAllowedHandler(func(w http.ResponseWriter, req bunrouter.Request) error {
			w.Header().Set("Allow", a.allow[req.Route()])
			w.WriteHeader(http.StatusMethodNotAllowed)
			return nil
		}),
	)
	return a
}

// handle registers a route and records its method for Allow. path is the
// full pattern, including any group prefix.
func (a *bunApp) handle(method, path string, h bunrouter.HandlerFunc) {
	a.Handle(method, path, h)
	if prev := a.allow[path]; prev != "" {
		a.allow[path] = prev + ", " + method
	} else {
		a.allow[path] = method
	}
}

// withoutBunRouter drops BunRouter from a 405 scenario on a parameter route.
// BunRouter answers a wrong method with 404, not 405, when a parameter is
// followed by more path segments (/teams/:id/users, for example), so it can't
// give the answer these scenarios check. A cheaper wrong answer isn't
// counted. Its static-route 405 is correct and stays measured.
func withoutBunRouter(cases []benchmarkCase) []benchmarkCase {
	out := cases[:0:0]
	for _, c := range cases {
		if c.name != "BunRouter" {
			out = append(out, c)
		}
	}
	return out
}

func bunText(s string) bunrouter.HandlerFunc {
	return func(w http.ResponseWriter, _ bunrouter.Request) error {
		writeText(w, s)
		return nil
	}
}

func buildBunRouterHelloHandler() http.Handler {
	a := newBunApp()
	a.handle(http.MethodGet, "/", bunText(benchmarkHelloResponse))
	return a
}

func buildBunRouterStaticHandler() http.Handler {
	a := newBunApp()
	a.handle(http.MethodGet, "/hello", bunText(benchmarkHelloResponse))
	return a
}

func buildBunRouterParamHandler() http.Handler {
	a := newBunApp()
	a.handle(http.MethodGet, "/hello/:name", func(w http.ResponseWriter, req bunrouter.Request) error {
		benchmarkSinkString = req.Param("name")
		writeText(w, benchmarkHelloResponse)
		return nil
	})
	return a
}

func buildBunRouterParallelParamHandler() http.Handler {
	a := newBunApp()
	a.handle(http.MethodGet, "/hello/:name", func(w http.ResponseWriter, req bunrouter.Request) error {
		if req.Param("name") == "" {
			w.WriteHeader(http.StatusInternalServerError)
			writeText(w, "BAD")
			return nil
		}
		writeText(w, benchmarkOKResponse)
		return nil
	})
	return a
}

func buildBunRouterLargeStaticHandler() http.Handler {
	a := newBunApp()
	for i := 0; i < largeStaticRouteCount; i++ {
		a.handle(http.MethodGet, largeStaticPath(i), bunText(benchmarkOKResponse))
	}
	return a
}

func buildBunRouterLargeParamHandler() http.Handler {
	a := newBunApp()
	for i := 0; i < largeParamRouteCount; i++ {
		a.handle(http.MethodGet, largeParamPatternColon(i), func(w http.ResponseWriter, req bunrouter.Request) error {
			benchmarkSinkString = req.Param("id")
			writeText(w, benchmarkOKResponse)
			return nil
		})
	}
	return a
}

func buildBunRouterMultiParamHandler(pattern string, paramNames []string) http.Handler {
	a := newBunApp()
	a.handle(http.MethodGet, pattern, func(w http.ResponseWriter, req bunrouter.Request) error {
		scenarioParamScoreBunRouter(paramNames, req)
		writeText(w, benchmarkOKResponse)
		return nil
	})
	return a
}

// buildBunRouterNestedGroupHandler registers the nested groups' routes by
// their full paths: a BunRouter group only prefixes its routes' paths.
func buildBunRouterNestedGroupHandler() http.Handler {
	a := newBunApp()
	ok := bunText(benchmarkOKResponse)
	a.handle(http.MethodGet, "/api/v1/health", ok)
	a.handle(http.MethodGet, "/api/v1/teams/status/health", ok)
	a.handle(http.MethodGet, "/api/v1/teams/:teamID", ok)
	a.handle(http.MethodGet, "/api/v1/teams/:teamID/users/", ok)
	a.handle(http.MethodGet, "/api/v1/teams/:teamID/users/:userID", func(w http.ResponseWriter, req bunrouter.Request) error {
		benchmarkSinkString = req.Param("teamID") + "|" + req.Param("userID")
		writeText(w, benchmarkOKResponse)
		return nil
	})
	a.handle(http.MethodGet, "/api/v1/teams/:teamID/users/:userID/preferences", ok)
	a.handle(http.MethodGet, "/api/v1/projects/:projectId/builds", ok)
	a.handle(http.MethodGet, "/api/v1/projects/:projectId/builds/:number", ok)
	return a
}

func buildBunRouterWildcardHandler() http.Handler {
	a := newBunApp()
	a.handle(http.MethodGet, "/files/*tail", func(w http.ResponseWriter, req bunrouter.Request) error {
		benchmarkSinkString = trimBenchmarkWildcard(req.Param("tail"))
		writeText(w, benchmarkOKResponse)
		return nil
	})
	return a
}

func scenarioParamScoreBunRouter(names []string, req bunrouter.Request) int {
	score := 0
	for _, name := range names {
		if name == "*" {
			score += len(strings.TrimPrefix(req.Param("tail"), "/"))
			continue
		}
		score += len(req.Param(name))
	}
	benchmarkSinkInt = score
	return score
}

func buildBunRouterScenarioHandler(routes []scenarioRoute) http.Handler {
	a := newBunApp()
	for _, route := range routes {
		paramNames := route.paramNames
		a.handle(route.method, route.ginPattern, func(w http.ResponseWriter, req bunrouter.Request) error {
			scenarioParamScoreBunRouter(paramNames, req)
			writeText(w, benchmarkOKResponse)
			return nil
		})
	}
	return a
}

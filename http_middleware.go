// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"net/http"

	"github.com/0mjs/zinc/internal/marks"
)

// FromHTTP adapts standard net/http middleware to Zinc middleware, so it can
// run on a group or a single route instead of the whole app:
//
//	admin := app.Group("/admin", zinc.FromHTTP(basicAuth))
//
// The middleware is built once. If it replaces the request or wraps the
// response writer before calling next, the rest of the Zinc chain uses the
// replacements. An error from that chain is written by the error handler
// before the standard middleware returns, so middleware that observes the
// response, such as a logger, sees its status; the error is still returned to
// outer Zinc middleware.
//
// A middleware that wraps the ResponseWriter must expose the writer it wraps
// through an Unwrap() http.ResponseWriter method, the convention
// http.ResponseController relies on. Zinc uses it to find the request's
// Context without allocating.
//
// On a group or route, the route has matched before the middleware runs, so
// the request carries it as http.ServeMux would set it: http.Request.PathValue
// returns each parameter and http.Request.Pattern is the route's
// "METHOD /path". On the app, through App.Use, the middleware runs before
// routing and sees neither.
func FromHTTP(middleware HTTPMiddleware) Middleware {
	if middleware == nil {
		panic("zinc: nil HTTP middleware")
	}
	handler := middleware(http.HandlerFunc(serveBridgedNext))
	if handler == nil {
		panic("zinc: HTTP middleware returned a nil handler")
	}
	return func(c *Context) error {
		outer := c.bridgeErr
		c.bridgeErr = nil
		c.publishRoute()
		handler.ServeHTTP(c.Writer(), c.Request())
		err := c.bridgeErr
		c.bridgeErr = outer
		return err
	}
}

// serveBridgedNext continues the Zinc chain from inside standard middleware.
func serveBridgedNext(w http.ResponseWriter, r *http.Request) {
	c := contextFromWriter(w)
	if c == nil {
		panic("zinc: FromHTTP middleware hid the response writer; its wrapper must implement Unwrap() http.ResponseWriter")
	}
	if r != c.request {
		// Keep request-derived caches when only the context changed, as when
		// middleware adds a tracing span; a new URL or body resets them.
		if c.request != nil && r.URL == c.request.URL && r.Body == c.request.Body {
			c.request = r
		} else {
			c.SetRequest(r)
		}
	}
	writer, baseWriter := c.writer, c.baseWriter
	if w != writer {
		c.SetWriter(w)
	}
	err := c.Next()
	if err != nil {
		// Write the error response now, through the middleware's writer, so
		// middleware that observes the response sees it, as it would with a
		// net/http handler. The error handler runs only once per request.
		c.HandleError(err)
	}
	c.bridgeErr = err
	c.writer, c.baseWriter = writer, baseWriter
}

// contextFromWriter follows Unwrap until it reaches a Zinc response writer.
func contextFromWriter(w http.ResponseWriter) *Context {
	for w != nil {
		if owned, ok := w.(interface{ contextOwner() *Context }); ok {
			return owned.contextOwner()
		}
		unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return nil
		}
		w = unwrapper.Unwrap()
	}
	return nil
}

// UseHTTP appends standard net/http middleware to the group. Each runs inside
// the group's earlier middleware, as FromHTTP describes.
func (g *Group) UseHTTP(middleware ...HTTPMiddleware) *Group {
	for _, mw := range middleware {
		g.Use(FromHTTP(mw))
	}
	return g
}

// Skip runs mw except when skip reports true for the request, in which case
// the chain continues without it. It replaces the Skipper option each
// middleware used to carry:
//
//	app.Use(zinc.Skip(isHealthCheck, logger.New()))
func Skip(skip func(*Context) bool, mw Middleware) Middleware {
	if skip == nil || mw == nil {
		panic("zinc: Skip needs a predicate and a middleware")
	}
	if middlewareMarks(mw) == (marks.Marks{}) {
		return func(c *Context) error {
			if skip(c) {
				return c.Next()
			}
			return mw(c)
		}
	}
	// Keep what Zinc knows about mw, such as rewrite running before
	// routing: the wrapper passes probes on to mw. Only marked middleware
	// gets this closure, so plain Skip wrappers are never probed.
	wrapped := func(c *Context) error {
		if c.request == marks.Probe {
			return mw(c)
		}
		if skip(c) {
			return c.Next()
		}
		return mw(c)
	}
	marks.Answers(wrapped)
	return wrapped
}

// middlewareMarks asks h what Zinc needs to know about it at registration.
// It calls h only when h's code answers probes, so other middleware never
// runs outside a request.
func middlewareMarks(h HandlerFunc) marks.Marks {
	if h == nil || !marks.CanAnswer(h) {
		return marks.Marks{}
	}
	if m, ok := h(&Context{request: marks.Probe}).(*marks.Marks); ok && m != nil {
		return *m
	}
	return marks.Marks{}
}

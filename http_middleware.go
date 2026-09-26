// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import "net/http"

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

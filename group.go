// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Group applies a shared path prefix and middleware chain to related routes.
type Group struct {
	app        *App
	prefix     string
	middleware []HandlerFunc
	// sealedBy describes the first registration that captured the middleware
	// chain. Middleware added after it would silently skip that registration.
	sealedBy string
}

// NewGroup creates a group bound to app.
func NewGroup(app *App, prefix string, handlers ...HandlerFunc) *Group {
	prefix = normalizeRegisteredPrefix(prefix)
	if prefix == "/" {
		prefix = ""
	}
	return &Group{
		app:        app,
		prefix:     prefix,
		middleware: append([]HandlerFunc(nil), handlers...),
	}
}

// Use appends middleware to the group. It panics once the group has routes,
// mounts, static files, or child groups, because those have already captured
// the chain and would silently run without the new middleware.
func (g *Group) Use(handlers ...HandlerFunc) *Group {
	if g.sealedBy != "" {
		prefix := g.prefix
		if prefix == "" {
			prefix = "/"
		}
		panic(fmt.Sprintf("zinc: Use on group %q after %s; register group middleware before its routes and child groups", prefix, g.sealedBy))
	}
	g.middleware = append(g.middleware, handlers...)
	return g
}

// seal records the first registration that captured the middleware chain.
// The description is built only once, so later registrations cost nothing.
func (g *Group) seal(kind, target string) {
	if g.sealedBy == "" {
		g.sealedBy = kind + " " + target
	}
}

// Group creates a child that inherits the parent's middleware in order.
func (g *Group) Group(prefix string, handlers ...HandlerFunc) *Group {
	fullPrefix := joinPaths(g.prefix, prefix)
	g.seal("child group", fullPrefix)
	sub := NewGroup(g.app, fullPrefix)
	sub.middleware = append(sub.middleware, g.middleware...)
	sub.middleware = append(sub.middleware, handlers...)
	return sub
}

// Route configures and returns a child group through fn.
func (g *Group) Route(prefix string, fn func(*Group), handlers ...HandlerFunc) *Group {
	sub := g.Group(prefix, handlers...)
	if fn != nil {
		fn(sub)
	}
	return sub
}

// Mount delegates a subtree below the group to a standard HTTP handler.
func (g *Group) Mount(prefix string, h http.Handler) {
	g.seal("mount", joinPaths(g.prefix, prefix))
	g.app.mount(joinPaths(g.prefix, prefix), h, g.middleware)
}

// Add registers handlers and panics when the route declaration is invalid.
func (g *Group) Add(method, routePath string, handlers ...HandlerFunc) {
	mustRegister(g.add(method, routePath, "", handlers...))
}

func (g *Group) add(method, routePath, name string, handlers ...HandlerFunc) error {
	fullPath := joinPaths(g.prefix, routePath)
	if g.sealedBy == "" {
		g.seal("route", method+" "+fullPath)
	}
	allHandlers := make([]HandlerFunc, 0, len(g.middleware)+len(handlers))
	allHandlers = append(allHandlers, g.middleware...)
	allHandlers = append(allHandlers, handlers...)
	return g.app.router.AddNamed(method, fullPath, name, allHandlers...)
}

// Handle registers a source-defined route below the group prefix and panics
// when its declaration is invalid.
func (g *Group) Handle(spec RouteSpec) {
	mustRegister(g.TryHandle(spec))
}

// TryHandle registers a dynamically defined route below the group prefix.
func (g *Group) TryHandle(spec RouteSpec) error {
	if spec.Handler == nil {
		return errors.New("route handler is nil")
	}
	return g.add(spec.Method, spec.Path, spec.Name, spec.Handler)
}

// HandleHTTP registers a standard net/http handler below the group prefix.
func (g *Group) HandleHTTP(pattern string, handler http.Handler) {
	if handler == nil {
		panic("zinc: HTTP handler is nil")
	}
	method, routePath, err := parseHTTPRoutePattern(pattern)
	if err != nil {
		panic(err)
	}
	g.Add(method, routePath, Wrap(handler))
}

// RouteNotFound registers a path-specific 404 handler below the group.
func (g *Group) RouteNotFound(routePath string, handlers ...HandlerFunc) {
	fullPath := joinPaths(g.prefix, routePath)
	g.seal("not-found route", fullPath)
	allHandlers := make([]HandlerFunc, 0, len(g.middleware)+len(handlers))
	allHandlers = append(allHandlers, g.middleware...)
	allHandlers = append(allHandlers, handlers...)
	g.app.RouteNotFound(fullPath, allHandlers...)
}

// Get registers a GET route.
func (g *Group) Get(path string, handlers ...HandlerFunc) {
	g.Add(MethodGet, path, handlers...)
}

// Post registers a POST route.
func (g *Group) Post(path string, handlers ...HandlerFunc) {
	g.Add(MethodPost, path, handlers...)
}

// Put registers a PUT route.
func (g *Group) Put(path string, handlers ...HandlerFunc) {
	g.Add(MethodPut, path, handlers...)
}

// Delete registers a DELETE route.
func (g *Group) Delete(path string, handlers ...HandlerFunc) {
	g.Add(MethodDelete, path, handlers...)
}

// Patch registers a PATCH route.
func (g *Group) Patch(path string, handlers ...HandlerFunc) {
	g.Add(MethodPatch, path, handlers...)
}

// Head registers a HEAD route.
func (g *Group) Head(path string, handlers ...HandlerFunc) {
	g.Add(MethodHead, path, handlers...)
}

// Options registers an OPTIONS route.
func (g *Group) Options(path string, handlers ...HandlerFunc) {
	g.Add(MethodOptions, path, handlers...)
}

// Connect registers a CONNECT route.
func (g *Group) Connect(path string, handlers ...HandlerFunc) {
	g.Add(MethodConnect, path, handlers...)
}

// Trace registers a TRACE route.
func (g *Group) Trace(path string, handlers ...HandlerFunc) {
	g.Add(MethodTrace, path, handlers...)
}

// Match registers the same handler chain for each method.
func (g *Group) Match(methods []string, routePath string, handlers ...HandlerFunc) {
	for _, method := range methods {
		g.Add(method, routePath, handlers...)
	}
}

// All registers handlers for Zinc's standard method set.
func (g *Group) All(path string, handlers ...HandlerFunc) {
	g.Match(routeMethods, path, handlers...)
}

// Any is an alias for All.
func (g *Group) Any(path string, handlers ...HandlerFunc) {
	g.All(path, handlers...)
}

// Static serves a filesystem directory below the group.
func (g *Group) Static(prefix, root string, opts ...StaticOption) error {
	filesystem := &confinedDirFS{path: root}
	if err := g.StaticFS(prefix, filesystem, opts...); err != nil {
		return err
	}
	g.app.staticRoots = append(g.app.staticRoots, filesystem)
	return nil
}

// StaticFS serves an fs.FS below the group.
func (g *Group) StaticFS(prefix string, filesystem fs.FS, opts ...StaticOption) error {
	g.seal("static files", joinPaths(g.prefix, prefix))
	return g.app.staticFS(joinPaths(g.prefix, prefix), filesystem, g.middleware, opts...)
}

// File serves one operating-system file below the group.
func (g *Group) File(routePath, file string) error {
	g.Get(routePath, func(c *Context) error { return c.File(file) })
	return nil
}

// FileFS serves one file from an fs.FS below the group.
func (g *Group) FileFS(routePath, file string, filesystem fs.FS) error {
	g.Get(routePath, func(c *Context) error { return c.FileFS(file, filesystem) })
	return nil
}

// joinPaths joins URL route prefixes without inheriting OS path semantics.
func joinPaths(a, b string) string {
	if b == "" {
		if a == "" {
			return "/"
		}
		return a
	}
	if !strings.HasPrefix(b, "/") {
		b = "/" + b
	}
	joined := path.Join("/", a, b)
	if strings.HasSuffix(b, "/") && joined != "/" {
		joined += "/"
	}
	return joined
}

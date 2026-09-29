// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
)

// ServeHTTP implements http.Handler. Standard HTTP middleware wraps the
// application here; Zinc middleware and routing continue through serveHTTP.
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.httpHandler != nil {
		a.httpHandler.ServeHTTP(w, r)
		return
	}
	a.serveHTTP(w, r)
}

// serveHTTP owns the pooled Context lifecycle. Every return path releases the
// Context only after handler errors have reached the configured error handler.
func (a *App) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if len(a.serverHeader) > 0 {
		w.Header().Set(HeaderServer, a.config.ServerHeader)
	}

	ctx := newContext(w, r)
	ctx.app = a

	defer ctx.release()
	if len(a.middlewareChain) > 0 {
		ctx.setHandlers(a.middlewareChain)
		if err := ctx.Next(); err != nil {
			a.handleError(ctx, err)
		}
		return
	}
	if err := appDispatchHandler(ctx); err != nil {
		a.handleError(ctx, err)
	}
}

// dispatch applies protocol-level routing policy around the router: automatic
// HEAD and OPTIONS, method mismatches, mounts, and route-specific 404 handlers.
func (a *App) dispatch(ctx *Context) error {
	method := ctx.Method()
	path := ctx.Path()
	needsAllowScan := (method == MethodOptions && a.autoOptions) || a.methodNotAllowed

	handled, allowed, err := a.router.dispatchInto(method, path, needsAllowScan, ctx)
	if !handled && method == MethodHead && a.autoHead {
		ctx.truncateParams(0)
		handled, allowed, err = a.router.dispatchInto(MethodGet, path, needsAllowScan, ctx)
	}
	if handled {
		return err
	}

	if mount := a.matchMount(path); mount != nil {
		ctx.setRoute(mount.info)
		if len(mount.chain) > 0 {
			ctx.setHandlers(mount.chain)
			return ctx.Next()
		}
		return mount.serve(ctx)
	}

	// The default spec is checked only after routing missed, so matched
	// requests pay nothing for it.
	if a.spec != nil && path == a.specPath && (method == MethodGet || method == MethodHead) {
		return a.spec.serve(ctx)
	}

	if handled, err := a.handleRouteNotFound(ctx); handled {
		return err
	}

	allowedHeader := allowed.header(a.autoHead, a.autoOptions)
	if method == MethodOptions && a.autoOptions && allowedHeader != "" {
		ctx.SetHeader(HeaderAllow, allowedHeader)
		return ctx.Status(StatusNoContent).NoContent()
	}

	if a.methodNotAllowed && allowedHeader != "" {
		ctx.Status(StatusMethodNotAllowed)
		// Allow and the default error's Content-Type share one allocation: the
		// spare capacity holds the Content-Type that writeDefaultMiss uses if
		// the error comes back unhandled. Appending to Allow overwrites it,
		// which writeDefaultMiss detects by the changed length.
		header := ctx.responseHeader()
		if a.defaultErrors {
			pair := []string{allowedHeader, jsonType}
			header[HeaderAllow] = pair[:1:2]
		} else {
			header[HeaderAllow] = []string{allowedHeader}
		}
		if a.methodNA != nil {
			if err := a.methodNA(ctx); err != nil {
				return err
			}
			if !ctx.written {
				return ctx.NoContent()
			}
			return nil
		}
		return ErrMethodNotAllowed
	}

	ctx.Status(StatusNotFound)
	if a.notFound != nil {
		if err := a.notFound(ctx); err != nil {
			return err
		}
		if !ctx.written {
			return ErrNotFound
		}
		return nil
	}
	return ErrNotFound
}

// handleRouteNotFound checks scoped not-found routes before the application
// fallback. The boolean distinguishes an executed handler from no match.
func (a *App) handleRouteNotFound(ctx *Context) (bool, error) {
	if a == nil || a.notFoundRoutes == nil || ctx == nil {
		return false, nil
	}
	handler := a.notFoundRoutes.findInto(MethodGet, ctx.Path(), ctx)
	if handler == nil {
		return false, nil
	}
	ctx.Status(StatusNotFound)
	if err := handler(ctx); err != nil {
		return true, err
	}
	if !ctx.written {
		return true, ErrNotFound
	}
	return true, nil
}

// handleError is the single terminal path for handler failures. Error handlers
// must not recursively return errors, so panics remain the recovery boundary.
func (a *App) handleError(ctx *Context, err error) {
	if err == nil {
		return
	}
	if ctx != nil {
		if ctx.errorHandled {
			return
		}
		ctx.errorHandled = true
		ctx.lastErr = err
		// Routing misses reach every middleware as ErrNotFound or
		// ErrMethodNotAllowed. When they come back unchanged to the default
		// handler, write its response directly: the bytes are the same.
		if a.defaultErrors && (err == ErrNotFound || err == ErrMethodNotAllowed) {
			ctx.writeDefaultMiss(err.(*HTTPError).Code)
			return
		}
	}
	a.config.ErrorHandler(ctx, err)
}

func appDispatchHandler(c *Context) error {
	if c != nil && c.app != nil {
		// Re-evaluate after each prefix chain: a rewrite can enter another
		// protected scope, including one registered earlier. Run each once.
		for i, entry := range c.app.prefixMiddleware {
			if !slices.Contains(c.prefixDone, i) && pathHasPrefix(c.Path(), entry.prefix, c.app.config.CaseSensitive) {
				c.prefixDone = append(c.prefixDone, i)
				c.setHandlers(entry.handlers)
				return c.Next()
			}
		}
		return c.app.dispatch(c)
	}
	return nil
}

// matchMount returns the first match from the longest-prefix-first mount list.
func (a *App) matchMount(path string) *mountedHandler {
	for i := range a.mounts {
		mount := &a.mounts[i]
		if pathHasPrefix(path, mount.prefix, a.config.CaseSensitive) {
			return mount
		}
	}
	return nil
}

// serve rewrites the request path for the mounted handler and restores it
// afterwards so outer middleware continues to observe the original request.
func (m *mountedHandler) serve(c *Context) error {
	if m == nil || (m.handler == nil && m.native == nil) {
		return nil
	}
	request := c.Request()
	if m.native != nil {
		// A native handler needs only the path below the prefix, so it
		// doesn't pay for a copy of the request.
		return m.native(c, request.URL.Path[m.prefixCut(request.URL.Path):])
	}
	m.handler.ServeHTTP(c.Writer(), m.strippedRequest(request))
	return nil
}

// prefixCut returns the byte length of the mount prefix in path. The mount
// was already matched using the application's case policy, so it counts
// prefix runes to find the boundary in the original (possibly Unicode)
// spelling.
func (m *mountedHandler) prefixCut(path string) int {
	cut := 0
	if m.prefixPath != "/" {
		for range m.prefixPath {
			if cut < len(path) {
				_, size := utf8.DecodeRuneInString(path[cut:])
				cut += size
			}
		}
	}
	return cut
}

// strippedRequest copies request with the mount prefix removed from its path.
func (m *mountedHandler) strippedRequest(request *http.Request) *http.Request {
	mountedRequest := new(http.Request)
	*mountedRequest = *request
	mountedRequest.URL = cloneURL(request.URL)
	// Map the prefix's boundary into the validated escaped path.
	cut := m.prefixCut(request.URL.Path)
	escaped := request.URL.EscapedPath()
	rawCut := 0
	for decoded := 0; decoded < cut && rawCut < len(escaped); decoded++ {
		if escaped[rawCut] == '%' {
			rawCut += 3
		} else {
			rawCut++
		}
	}
	mountedRequest.URL.Path = request.URL.Path[cut:]
	mountedRequest.URL.RawPath = escaped[rawCut:]
	if !strings.HasPrefix(mountedRequest.URL.RawPath, "/") {
		mountedRequest.URL.RawPath = ""
	}
	if mountedRequest.URL.Path == "" {
		mountedRequest.URL.Path = "/"
		mountedRequest.URL.RawPath = "/"
	}
	mountedRequest.RequestURI = cloneRequestURI(mountedRequest.URL)
	return mountedRequest
}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"net/http"
	"strings"
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
		w.Header()[HeaderServer] = a.serverHeader
	}

	ctx := NewContext(w, r)
	ctx.app = a

	if len(a.middlewareChain) > 0 && len(a.prefixMiddleware) == 0 {
		ctx.setHandlers(a.middlewareChain)
		if err := ctx.Next(); err != nil {
			a.handleError(ctx, err)
		}
		ctx.release()
		return
	}

	if len(a.middleware) > 0 || len(a.prefixMiddleware) > 0 {
		handlers := a.preHandlersForPath(r.URL.Path)
		if len(handlers) > 0 {
			ctx.setHandlers(handlers)
			if err := ctx.Next(); err != nil {
				a.handleError(ctx, err)
			}
			ctx.release()
			return
		}
	}

	if err := a.dispatch(ctx); err != nil {
		a.handleError(ctx, err)
	}
	ctx.release()
}

// preHandlersForPath builds a per-request chain only when prefix middleware is
// configured. Applications with global middleware use the prebuilt chain.
func (a *App) preHandlersForPath(path string) []HandlerFunc {
	count := len(a.middleware)
	for _, entry := range a.prefixMiddleware {
		if pathHasPrefix(path, entry.prefix, a.config.CaseSensitive) {
			count += len(entry.handlers)
		}
	}
	if count == 0 {
		return nil
	}
	handlers := make([]HandlerFunc, 0, count+1)
	handlers = append(handlers, a.middleware...)
	for _, entry := range a.prefixMiddleware {
		if pathHasPrefix(path, entry.prefix, a.config.CaseSensitive) {
			handlers = append(handlers, entry.handlers...)
		}
	}
	handlers = append(handlers, appDispatchHandler)
	return handlers
}

// dispatch applies protocol-level routing policy around the router: automatic
// HEAD and OPTIONS, method mismatches, mounts, and route-specific 404 handlers.
func (a *App) dispatch(ctx *Context) error {
	method := ctx.Method()
	path := ctx.Path()
	needsAllowScan := (method == MethodOptions && a.config.AutoOptions) || a.config.HandleMethodNotAllowed

	handled, allowed, err := a.router.dispatchInto(method, path, needsAllowScan, ctx)
	if !handled && method == MethodHead && a.config.AutoHead {
		ctx.truncateParams(0)
		handled, allowed, err = a.router.dispatchInto(MethodGet, path, needsAllowScan, ctx)
	}
	if handled {
		return err
	}

	if mount := a.matchMount(path); mount != nil {
		ctx.setRoute(mount.info)
		mount.serve(ctx)
		return nil
	}

	if handled, err := a.handleRouteNotFound(ctx); handled {
		return err
	}

	allowedHeader := allowed.header(a.config.AutoHead, a.config.AutoOptions)
	if method == MethodOptions && a.config.AutoOptions && allowedHeader != "" {
		ctx.SetHeader(HeaderAllow, allowedHeader)
		return ctx.Status(StatusNoContent).NoContent()
	}

	if a.config.HandleMethodNotAllowed && allowedHeader != "" {
		if a.methodNA == nil && a.defaultErrors {
			return ctx.writeDefaultErrorResponse(StatusMethodNotAllowed, allowedHeader)
		}
		ctx.Status(StatusMethodNotAllowed)
		ctx.SetHeader(HeaderAllow, allowedHeader)
		if a.methodNA != nil {
			if err := a.methodNA(ctx); err != nil {
				return err
			}
			if !ctx.written {
				return ctx.NoContent()
			}
			return nil
		}
		if a.defaultErrors {
			return ctx.String(http.StatusText(StatusMethodNotAllowed))
		}
		return ErrMethodNotAllowed
	}

	if a.notFound == nil && a.defaultErrors {
		return ctx.writeDefaultErrorResponse(StatusNotFound, "")
	}

	ctx.Status(StatusNotFound)
	if a.notFound != nil {
		if err := a.notFound(ctx); err != nil {
			return err
		}
		if !ctx.written {
			return ctx.String(http.StatusText(StatusNotFound))
		}
		return nil
	}
	if a.defaultErrors {
		return ctx.String(http.StatusText(StatusNotFound))
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
		return true, ctx.String(http.StatusText(StatusNotFound))
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
		ctx.lastErr = err
	}
	a.config.ErrorHandler(ctx, err)
}

func appDispatchHandler(c *Context) error {
	if c != nil && c.app != nil {
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
func (m *mountedHandler) serve(c *Context) {
	if m == nil || m.handler == nil {
		return
	}
	request := c.Request()
	mountedRequest := new(http.Request)
	*mountedRequest = *request
	mountedRequest.URL = cloneURL(request.URL)
	mountedRequest.RequestURI = cloneRequestURI(mountedRequest.URL)
	mountedRequest.URL.Path = stripMountPrefix(mountedRequest.URL.Path, m.prefixPath)
	if mountedRequest.URL.RawPath != "" {
		mountedRequest.URL.RawPath = stripMountPrefix(mountedRequest.URL.RawPath, m.prefixPath)
	}
	if mountedRequest.URL.Path == "" {
		mountedRequest.URL.Path = "/"
	}
	if mountedRequest.URL.RawPath == "" && mountedRequest.URL.Path != "" {
		mountedRequest.URL.RawPath = mountedRequest.URL.Path
	}
	m.handler.ServeHTTP(c.Writer(), mountedRequest)
	c.written = true
}

func stripMountPrefix(path, prefix string) string {
	if prefix == "/" {
		return path
	}
	trimmed := strings.TrimPrefix(path, prefix)
	if trimmed == "" {
		return "/"
	}
	if trimmed[0] != '/' {
		return "/" + trimmed
	}
	return trimmed
}

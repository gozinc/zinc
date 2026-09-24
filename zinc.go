// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"net/http"
	"slices"
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
		w.Header().Set(HeaderServer, a.config.ServerHeader)
	}

	ctx := NewContext(w, r)
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
		if len(mount.chain) > 0 {
			ctx.setHandlers(mount.chain)
			return ctx.Next()
		}
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
		if ctx.errorHandled {
			return
		}
		ctx.errorHandled = true
		ctx.lastErr = err
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

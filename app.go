// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// DefaultListenAddr is used when Listen receives no address or an empty one.
const DefaultListenAddr = ":8080"

// Map is a concise map type for dynamic JSON and template data.
type Map map[string]any

// HandlerFunc handles one request and returns failures to the App error handler.
type HandlerFunc func(*Context) error

// HTTPMiddleware is the standard net/http middleware shape.
type HTTPMiddleware func(http.Handler) http.Handler

type httpHandlerSlot struct {
	handler http.Handler
}

func (s *httpHandlerSlot) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// RouteHandler is retained as a descriptive alias for HandlerFunc.
type RouteHandler = HandlerFunc

// Middleware is a HandlerFunc that calls Context.Next to continue the chain.
type Middleware = HandlerFunc

// RouteInfo is an immutable snapshot of registered route metadata.
type RouteInfo struct {
	Name    string
	Method  string
	Path    string
	Params  []string
	Mounted bool
	Handler string
}

// RouteSpec describes a route supplied by configuration, plugins, or generated code.
type RouteSpec struct {
	Name    string
	Method  string
	Path    string
	Handler HandlerFunc
}

type routeMeta struct {
	name      string
	method    string
	path      string
	params    []string
	mounted   bool
	handlerPC uintptr
}

func (m routeMeta) export() RouteInfo {
	return RouteInfo{
		Name:    m.name,
		Method:  m.method,
		Path:    m.path,
		Params:  append([]string(nil), m.params...),
		Mounted: m.mounted,
		Handler: handlerNameFromPC(m.handlerPC),
	}
}

func (m routeMeta) url(values []string) (string, error) {
	if len(m.params) != len(values) {
		return "", fmt.Errorf("route %q expects %d params, got %d", m.name, len(m.params), len(values))
	}
	if len(m.params) == 0 {
		return m.path, nil
	}

	var builder strings.Builder
	valueIndex := 0
	for i := 0; i < len(m.path); i++ {
		switch m.path[i] {
		case '{':
			endOffset := strings.IndexByte(m.path[i+1:], '}')
			if endOffset < 0 {
				return "", fmt.Errorf("invalid route pattern %q", m.path)
			}
			end := i + 1 + endOffset
			wildcard := strings.HasSuffix(m.path[i+1:end], "...")
			if wildcard {
				builder.WriteString(values[valueIndex])
			} else {
				builder.WriteString(url.PathEscape(values[valueIndex]))
			}
			valueIndex++
			i = end
		default:
			builder.WriteByte(m.path[i])
		}
	}
	return builder.String(), nil
}

func newRouteMeta(method, path, name string, handler HandlerFunc, params []string, mounted bool) routeMeta {
	return routeMeta{
		name:      name,
		method:    method,
		path:      path,
		params:    append([]string(nil), params...),
		mounted:   mounted,
		handlerPC: handlerPC(handler),
	}
}

type prefixMiddleware struct {
	prefix   string
	handlers []HandlerFunc
}

type mountedHandler struct {
	prefix     string
	prefixPath string
	handler    http.Handler
	info       routeMeta
}

// App is Zinc's application, router, middleware registry, and http.Handler.
// Configure routes and middleware before serving; registration is not safe
// concurrently with requests.
type App struct {
	config           Config
	router           *Router
	notFoundRoutes   *Router
	middleware       []HandlerFunc
	middlewareChain  []HandlerFunc
	httpHandler      http.Handler
	httpHandlerTail  *httpHandlerSlot
	prefixMiddleware []prefixMiddleware
	mounts           []mountedHandler
	notFound         HandlerFunc
	methodNA         HandlerFunc
	server           *http.Server
	serverMu         sync.Mutex
	serverHeader     []string
	defaultErrors    bool
}

// New creates an App from DefaultConfig.
func New() *App {
	return NewWithConfig(DefaultConfig)
}

// NewWithConfig creates an App and fills zero-valued service dependencies and
// server limits from DefaultConfig.
func NewWithConfig(cfg Config) *App {
	defaultErrors := cfg.ErrorHandler == nil
	cfg = normalizeConfig(cfg)

	var cache *RouteCache
	if cfg.RouteCacheSize > 0 {
		cache = NewRouteCache(cfg.RouteCacheSize)
	}

	app := &App{
		config: cfg,
		router: &Router{
			cache:  cache,
			config: &cfg,
		},
		middleware:    make([]HandlerFunc, 0),
		defaultErrors: defaultErrors,
	}
	if cfg.ServerHeader != "" {
		app.serverHeader = []string{cfg.ServerHeader}
	}
	return app
}

func normalizeConfig(cfg Config) Config {
	if cfg.BodyLimit == 0 {
		cfg.BodyLimit = DefaultConfig.BodyLimit
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = DefaultConfig.ReadTimeout
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = DefaultConfig.WriteTimeout
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = DefaultConfig.IdleTimeout
	}
	if cfg.ProxyHeader == "" {
		cfg.ProxyHeader = DefaultConfig.ProxyHeader
	}
	if cfg.JSONCodec == nil {
		cfg.JSONCodec = defaultJSONCodec{}
	}
	if cfg.RequestBinder == nil {
		cfg.RequestBinder = defaultBinder{codec: cfg.JSONCodec}
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaultErrorHandler
	}
	return cfg
}

// Handler returns the App as a standard-library handler.
func (a *App) Handler() http.Handler {
	return a
}

// Listen starts the app on addr, defaulting to :8080 when addr is omitted.
func (a *App) Listen(addr ...string) error {
	listenAddr, err := resolveListenAddr(addr...)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	return a.Serve(ln)
}

func resolveListenAddr(addr ...string) (string, error) {
	switch len(addr) {
	case 0:
		return DefaultListenAddr, nil
	case 1:
		if addr[0] == "" {
			return DefaultListenAddr, nil
		}
		return addr[0], nil
	default:
		return "", fmt.Errorf("expected at most one listen address, got %d", len(addr))
	}
}

// ListenTLS starts an HTTPS server using certificate and key files.
func (a *App) ListenTLS(addr, certFile, keyFile string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return a.serveTLS(ln, certFile, keyFile)
}

// Serve accepts HTTP requests from an existing listener.
func (a *App) Serve(ln net.Listener) error {
	if ln == nil {
		return errors.New("listener is nil")
	}
	return a.serve(ln, "", "")
}

func (a *App) serveTLS(ln net.Listener, certFile, keyFile string) error {
	if ln == nil {
		return errors.New("listener is nil")
	}
	return a.serve(ln, certFile, keyFile)
}

func (a *App) serve(ln net.Listener, certFile, keyFile string) error {
	srv := &http.Server{
		Handler:      a,
		ReadTimeout:  a.config.ReadTimeout,
		WriteTimeout: a.config.WriteTimeout,
		IdleTimeout:  a.config.IdleTimeout,
	}

	a.serverMu.Lock()
	a.server = srv
	a.serverMu.Unlock()

	var err error
	if certFile != "" || keyFile != "" {
		err = srv.ServeTLS(ln, certFile, keyFile)
	} else {
		err = srv.Serve(ln)
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gracefully stops the active server. It is a no-op before serving.
func (a *App) Shutdown(ctx context.Context) error {
	a.serverMu.Lock()
	srv := a.server
	a.serverMu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// Use appends Zinc middleware in registration order.
func (a *App) Use(handlers ...HandlerFunc) {
	a.middleware = append(a.middleware, handlers...)
	a.rebuildMiddlewareChain()
}

// UseHTTP wraps the whole application in standard net/http middleware.
// Middleware runs in registration order, with the first middleware outermost.
func (a *App) UseHTTP(middleware ...HTTPMiddleware) {
	if len(middleware) == 0 {
		return
	}

	tail := &httpHandlerSlot{handler: http.HandlerFunc(a.serveHTTP)}
	var handler http.Handler = tail

	for i := len(middleware) - 1; i >= 0; i-- {
		nextMiddleware := middleware[i]
		if nextMiddleware == nil {
			panic("zinc: nil HTTP middleware")
		}
		handler = nextMiddleware(handler)
		if handler == nil {
			panic("zinc: HTTP middleware returned a nil handler")
		}
	}

	if a.httpHandler == nil {
		a.httpHandler = handler
	} else {
		a.httpHandlerTail.handler = handler
	}
	a.httpHandlerTail = tail
}

// UsePrefix applies Zinc middleware only to requests below a segment boundary.
func (a *App) UsePrefix(prefix string, handlers ...HandlerFunc) {
	prefix = normalizeRegisteredPrefix(prefix)
	a.prefixMiddleware = append(a.prefixMiddleware, prefixMiddleware{prefix: prefix, handlers: append([]HandlerFunc(nil), handlers...)})
}

// rebuildMiddlewareChain keeps the global-only request path allocation-free.
func (a *App) rebuildMiddlewareChain() {
	if len(a.middleware) == 0 {
		a.middlewareChain = nil
		return
	}
	chain := make([]HandlerFunc, len(a.middleware)+1)
	copy(chain, a.middleware)
	chain[len(chain)-1] = appDispatchHandler
	a.middlewareChain = chain
}

// Group creates a route group rooted at prefix.
func (a *App) Group(prefix string, handlers ...HandlerFunc) *Group {
	return NewGroup(a, prefix, handlers...)
}

// Route configures and returns a group through fn.
func (a *App) Route(prefix string, fn func(*Group), handlers ...HandlerFunc) *Group {
	group := a.Group(prefix, handlers...)
	if fn != nil {
		fn(group)
	}
	return group
}

// Mount delegates a path subtree to h. More specific mounts take precedence.
func (a *App) Mount(prefix string, h http.Handler) {
	prefix = normalizeRegisteredPrefix(prefix)
	entry := mountedHandler{
		prefix:     storedPrefix(prefix, a.config.CaseSensitive),
		prefixPath: prefix,
		handler:    h,
		info:       newRouteMeta(methodUse, prefix, "", Wrap(h), nil, true),
	}
	a.mounts = append(a.mounts, entry)
	sort.SliceStable(a.mounts, func(i, j int) bool {
		return len(a.mounts[i].prefixPath) > len(a.mounts[j].prefixPath)
	})
}

// AcquireContext creates an application-bound Context for advanced integrations.
// The caller must eventually pass it to ReleaseContext.
func (a *App) AcquireContext(w http.ResponseWriter, r *http.Request) *Context {
	ctx := NewContext(w, r)
	ctx.app = a
	return ctx
}

// ReleaseContext returns a manually acquired Context to its pool.
func (a *App) ReleaseContext(c *Context) {
	if c == nil {
		return
	}
	c.release()
}

// NotFound replaces the application-wide 404 handler.
func (a *App) NotFound(handler HandlerFunc) {
	a.notFound = handler
}

// RouteNotFound registers a path-specific handler used when no method matches.
func (a *App) RouteNotFound(path string, handlers ...HandlerFunc) {
	if len(handlers) == 0 {
		panic("zinc: route handler is nil")
	}
	if a.notFoundRoutes == nil {
		a.notFoundRoutes = &Router{config: &a.config}
	}
	mustRegister(a.notFoundRoutes.Add(MethodGet, path, handlers...))
}

// MethodNotAllowed replaces the application-wide 405 handler.
func (a *App) MethodNotAllowed(handler HandlerFunc) {
	a.methodNA = handler
}

// Routes returns registered routes and mounts in registration order.
func (a *App) Routes() []RouteInfo {
	routes := a.router.Routes()
	if len(a.mounts) == 0 {
		return routes
	}
	out := make([]RouteInfo, 0, len(routes)+len(a.mounts))
	out = append(out, routes...)
	for _, mount := range a.mounts {
		out = append(out, mount.info.export())
	}
	return out
}

// Handle registers a source-defined route and panics when its declaration is invalid.
func (a *App) Handle(spec RouteSpec) {
	mustRegister(a.TryHandle(spec))
}

// TryHandle registers a route whose declaration came from dynamic input.
func (a *App) TryHandle(spec RouteSpec) error {
	if spec.Handler == nil {
		return errors.New("route handler is nil")
	}
	return a.router.AddNamed(spec.Method, spec.Path, spec.Name, spec.Handler)
}

// HandleHTTP registers a standard net/http handler using a "METHOD /path"
// pattern, such as "GET /metrics". Matched parameters are available through
// http.Request.PathValue inside the standard handler.
func (a *App) HandleHTTP(pattern string, handler http.Handler) {
	if handler == nil {
		panic("zinc: HTTP handler is nil")
	}
	method, path, err := parseHTTPRoutePattern(pattern)
	if err != nil {
		panic(err)
	}
	mustRegister(a.router.Add(method, path, Wrap(handler)))
}

func mustRegister(err error) {
	if err != nil {
		panic(err)
	}
}

func parseHTTPRoutePattern(pattern string) (string, string, error) {
	parts := strings.Fields(pattern)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid HTTP route pattern %q: expected METHOD /path", pattern)
	}
	method, path := parts[0], parts[1]
	if method == "" {
		return "", "", fmt.Errorf("invalid HTTP route pattern %q: method is empty", pattern)
	}
	if !strings.HasPrefix(path, "/") {
		return "", "", fmt.Errorf("invalid HTTP route pattern %q: path must start with /", pattern)
	}
	return method, path, nil
}

// RouteByName finds route metadata by its unique name.
func (a *App) RouteByName(name string) (RouteInfo, bool) {
	meta, ok := a.router.routeMetaByName(name)
	if !ok {
		return RouteInfo{}, false
	}
	return meta.export(), true
}

// URL builds a named route URL. Segment parameters are escaped; catch-all
// values are inserted verbatim so callers can supply a complete suffix.
func (a *App) URL(name string, params ...string) (string, error) {
	meta, ok := a.router.routeMetaByName(name)
	if !ok {
		return "", fmt.Errorf("route %q not found", name)
	}
	return meta.url(params)
}

// FindRoute resolves metadata without invoking the route handler.
func (a *App) FindRoute(method, path string) (RouteInfo, bool) {
	_, ctx := a.router.Find(method, path)
	if ctx != nil {
		return ctx.Route(), true
	}
	if mount := a.matchMount(path); mount != nil {
		return mount.info.export(), true
	}
	return RouteInfo{}, false
}

// RoutesByMethod returns route metadata registered for method.
func (a *App) RoutesByMethod(method string) []RouteInfo {
	routes := a.Routes()
	out := make([]RouteInfo, 0, len(routes))
	for _, route := range routes {
		if route.Method == method {
			out = append(out, route)
		}
	}
	return out
}

// RoutesByPrefix returns route metadata below prefix.
func (a *App) RoutesByPrefix(prefix string) []RouteInfo {
	prefix = normalizeRegisteredPrefix(prefix)
	routes := a.Routes()
	out := make([]RouteInfo, 0, len(routes))
	for _, route := range routes {
		if strings.HasPrefix(route.Path, prefix) {
			out = append(out, route)
		}
	}
	return out
}

// Wrap adapts a standard net/http handler to HandlerFunc. Matched parameters
// are populated into http.Request.PathValue immediately before it runs.
func Wrap(h http.Handler) HandlerFunc {
	return func(c *Context) error {
		c.populateRequestPathValues()
		h.ServeHTTP(c.Writer(), c.Request())
		c.written = true
		return nil
	}
}

// WrapFunc adapts a standard http.HandlerFunc to HandlerFunc.
func WrapFunc(fn http.HandlerFunc) HandlerFunc {
	return Wrap(fn)
}

func normalizeRegisteredPrefix(prefix string) string {
	if prefix == "" {
		return "/"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if len(prefix) > 1 {
		prefix = strings.TrimSuffix(prefix, "/")
		if prefix == "" {
			return "/"
		}
	}
	return prefix
}

func storedPrefix(prefix string, caseSensitive bool) string {
	if caseSensitive {
		return prefix
	}
	return strings.ToLower(prefix)
}

func pathHasPrefix(path, prefix string, caseSensitive bool) bool {
	if !caseSensitive {
		path = strings.ToLower(path)
		prefix = strings.ToLower(prefix)
	}
	if prefix == "/" {
		return true
	}
	if path == prefix {
		return true
	}
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	if strings.HasSuffix(prefix, "/") {
		return true
	}
	return len(path) > len(prefix) && path[len(prefix)] == '/'
}

var handlerNameCache sync.Map

func handlerPC(handler HandlerFunc) uintptr {
	// Program counters are retained for diagnostics only and never participate in dispatch.
	if handler == nil {
		return 0
	}
	value := reflect.ValueOf(handler)
	if !value.IsValid() || value.IsNil() {
		return 0
	}
	return value.Pointer()
}

func handlerName(handler HandlerFunc) string {
	return handlerNameFromPC(handlerPC(handler))
}

func handlerNameFromPC(pc uintptr) string {
	if pc == 0 {
		return ""
	}
	if cached, ok := handlerNameCache.Load(pc); ok {
		return cached.(string)
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return ""
	}
	name := fn.Name()
	handlerNameCache.Store(pc, name)
	return name
}

// Add registers handlers and panics when the route declaration is invalid.
func (a *App) Add(method, path string, handlers ...HandlerFunc) {
	mustRegister(a.router.Add(method, path, handlers...))
}

// Get registers a GET route.
func (a *App) Get(path string, handlers ...HandlerFunc) {
	a.Add(MethodGet, path, handlers...)
}

// Post registers a POST route.
func (a *App) Post(path string, handlers ...HandlerFunc) {
	a.Add(MethodPost, path, handlers...)
}

// Put registers a PUT route.
func (a *App) Put(path string, handlers ...HandlerFunc) {
	a.Add(MethodPut, path, handlers...)
}

// Delete registers a DELETE route.
func (a *App) Delete(path string, handlers ...HandlerFunc) {
	a.Add(MethodDelete, path, handlers...)
}

// Patch registers a PATCH route.
func (a *App) Patch(path string, handlers ...HandlerFunc) {
	a.Add(MethodPatch, path, handlers...)
}

// Head registers a HEAD route.
func (a *App) Head(path string, handlers ...HandlerFunc) {
	a.Add(MethodHead, path, handlers...)
}

// Options registers an OPTIONS route.
func (a *App) Options(path string, handlers ...HandlerFunc) {
	a.Add(MethodOptions, path, handlers...)
}

// Connect registers a CONNECT route.
func (a *App) Connect(path string, handlers ...HandlerFunc) {
	a.Add(MethodConnect, path, handlers...)
}

// Trace registers a TRACE route.
func (a *App) Trace(path string, handlers ...HandlerFunc) {
	a.Add(MethodTrace, path, handlers...)
}

// Match registers the same handler chain for each method.
func (a *App) Match(methods []string, path string, handlers ...HandlerFunc) {
	for _, method := range methods {
		a.Add(method, path, handlers...)
	}
}

// All registers handlers for Zinc's standard method set.
func (a *App) All(path string, handlers ...HandlerFunc) {
	a.Match(routeMethods, path, handlers...)
}

// Any is an alias for All.
func (a *App) Any(path string, handlers ...HandlerFunc) {
	a.All(path, handlers...)
}

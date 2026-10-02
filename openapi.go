// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// OpenAPIConfig describes the API as a whole in the OpenAPI spec. Every field
// is optional.
type OpenAPIConfig struct {
	// Title names the API. It defaults to the last element of the main
	// module's path, or "API".
	Title string
	// Version is the API's version, not Zinc's. It defaults to the main
	// module's version, or "0.0.0" for a development build.
	Version string
	// Description may use Markdown.
	Description string
	// TermsOfService is a URL.
	TermsOfService string
	Contact        *OpenAPIContact
	License        *OpenAPILicense
	// ExternalDocs links to documentation beyond the spec.
	ExternalDocs *OpenAPIExternalDocs
	// Servers lists base URLs the API is served from.
	Servers []OpenAPIServer
	// Tags describes tags, in the order docs pages list them. Tags that
	// routes use but Tags leaves out follow, in the order routes use them.
	Tags []OpenAPITag
	// SecuritySchemes defines the schemes Route.Security, Group.Security and
	// Security refer to, by name.
	SecuritySchemes map[string]OpenAPISecurityScheme
	// Security names the schemes that protect every route that doesn't set
	// its own with Route.Security or Group.Security. Any one of them is
	// enough. A name can carry a scope: "oauth:pets:read".
	Security []string
	// NoAuthResponses leaves out the 401, and the 403 for a route that needs
	// scopes, that Zinc adds to every secured route.
	NoAuthResponses bool
	// Schemas gives a schema to a type Zinc can't describe and you can't add
	// a SchemaProvider to, such as a type from another module:
	//
	//	Schemas: map[reflect.Type]map[string]any{
	//		reflect.TypeFor[decimal.Decimal](): {"type": "string", "format": "decimal"},
	//	}
	Schemas map[reflect.Type]map[string]any
}

// OpenAPIContact is who to contact about the API.
type OpenAPIContact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

// OpenAPILicense is the API's license. Name is required; set Identifier, an
// SPDX expression such as "MIT", or URL, not both.
type OpenAPILicense struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier,omitempty"`
	URL        string `json:"url,omitempty"`
}

// OpenAPIExternalDocs links to documentation outside the spec.
type OpenAPIExternalDocs struct {
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
}

// OpenAPITag describes a tag routes use.
type OpenAPITag struct {
	Name         string               `json:"name"`
	Description  string               `json:"description,omitempty"`
	ExternalDocs *OpenAPIExternalDocs `json:"externalDocs,omitempty"`
}

// OpenAPIServer is a base URL the API is served from.
type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// OpenAPISecurityScheme describes one way a client authenticates.
//
//	zinc.OpenAPISecurityScheme{Type: "http", Scheme: "bearer", BearerFormat: "JWT"}
//	zinc.OpenAPISecurityScheme{Type: "apiKey", In: "header", Name: "X-API-Key"}
//	zinc.OpenAPISecurityScheme{Type: "oauth2", Flows: &zinc.OpenAPIOAuthFlows{
//		ClientCredentials: &zinc.OpenAPIOAuthFlow{TokenURL: "https://id.example.com/token"},
//	}}
//
// A scheme missing a field its type needs, such as an "oauth2" scheme without
// flows, makes the spec fail to build.
type OpenAPISecurityScheme struct {
	// Type is "http", "apiKey", "oauth2", "openIdConnect" or "mutualTLS".
	Type string `json:"type"`
	// Scheme is the HTTP authentication scheme for type "http", such as
	// "bearer" or "basic".
	Scheme string `json:"scheme,omitempty"`
	// BearerFormat hints at the token format for scheme "bearer", such as "JWT".
	BearerFormat string `json:"bearerFormat,omitempty"`
	// In is where an "apiKey" is sent: "header", "query" or "cookie".
	In string `json:"in,omitempty"`
	// Name is the header, query parameter or cookie that carries an "apiKey".
	Name string `json:"name,omitempty"`
	// Flows lists the flows an "oauth2" scheme supports; it needs at least
	// one.
	Flows *OpenAPIOAuthFlows `json:"flows,omitempty"`
	// OpenIDConnectURL is the discovery URL for type "openIdConnect".
	OpenIDConnectURL string `json:"openIdConnectUrl,omitempty"`
	Description      string `json:"description,omitempty"`
}

// OpenAPIOAuthFlows lists the OAuth 2 flows a scheme supports. Set at least
// one.
type OpenAPIOAuthFlows struct {
	// AuthorizationCode needs AuthorizationURL and TokenURL.
	AuthorizationCode *OpenAPIOAuthFlow `json:"authorizationCode,omitempty"`
	// ClientCredentials needs TokenURL.
	ClientCredentials *OpenAPIOAuthFlow `json:"clientCredentials,omitempty"`
	// Password needs TokenURL.
	Password *OpenAPIOAuthFlow `json:"password,omitempty"`
	// Implicit needs AuthorizationURL.
	Implicit *OpenAPIOAuthFlow `json:"implicit,omitempty"`
}

// OpenAPIOAuthFlow is one OAuth 2 flow.
type OpenAPIOAuthFlow struct {
	AuthorizationURL string `json:"authorizationUrl,omitempty"`
	TokenURL         string `json:"tokenUrl,omitempty"`
	RefreshURL       string `json:"refreshUrl,omitempty"`
	// Scopes maps each scope the flow grants to a short description.
	Scopes map[string]string `json:"scopes"`
}

// checkSecuritySchemes reports the first scheme missing a field its type
// needs, and returns the schemes as the spec writes them.
func checkSecuritySchemes(schemes map[string]OpenAPISecurityScheme) (map[string]OpenAPISecurityScheme, error) {
	out := make(map[string]OpenAPISecurityScheme, len(schemes))
	for name, s := range schemes {
		bad := func(format string, args ...any) error {
			return fmt.Errorf("zinc: security scheme %q: %s", name, fmt.Sprintf(format, args...))
		}
		switch s.Type {
		case "http":
			if s.Scheme == "" {
				return nil, bad(`type "http" needs Scheme, such as "bearer"`)
			}
		case "apiKey":
			if s.Name == "" || (s.In != "header" && s.In != "query" && s.In != "cookie") {
				return nil, bad(`type "apiKey" needs Name, and In set to "header", "query" or "cookie"`)
			}
		case "openIdConnect":
			if s.OpenIDConnectURL == "" {
				return nil, bad(`type "openIdConnect" needs OpenIDConnectURL`)
			}
		case "mutualTLS":
		case "oauth2":
			f := s.Flows
			if f == nil || (f.AuthorizationCode == nil && f.ClientCredentials == nil && f.Password == nil && f.Implicit == nil) {
				return nil, bad(`type "oauth2" needs Flows with at least one flow`)
			}
			flows := *f
			for _, flow := range []struct {
				name        string
				flow        **OpenAPIOAuthFlow
				auth, token bool
			}{
				{"AuthorizationCode", &flows.AuthorizationCode, true, true},
				{"ClientCredentials", &flows.ClientCredentials, false, true},
				{"Password", &flows.Password, false, true},
				{"Implicit", &flows.Implicit, true, false},
			} {
				fl := *flow.flow
				if fl == nil {
					continue
				}
				if flow.auth && fl.AuthorizationURL == "" {
					return nil, bad("the %s flow needs AuthorizationURL", flow.name)
				}
				if flow.token && fl.TokenURL == "" {
					return nil, bad("the %s flow needs TokenURL", flow.name)
				}
				if fl.Scopes == nil {
					// OpenAPI requires the scopes object, even when empty.
					c := *fl
					c.Scopes = map[string]string{}
					*flow.flow = &c
				}
			}
			s.Flows = &flows
		default:
			return nil, bad(`unknown type %q; want "http", "apiKey", "oauth2", "openIdConnect" or "mutualTLS"`, s.Type)
		}
		out[name] = s
	}
	return out, nil
}

// OpenAPISpec returns the app's OpenAPI 3.1 spec as JSON. Every registered
// route is in it, except hidden ones, mounts and static files, and methods
// OpenAPI 3.1 can't express, such as PURGE. Typed handlers are described
// fully; other handlers add detail with Route.Input, Route.Output and the
// other Route methods. The output is the same for the same routes.
func (a *App) OpenAPISpec(cfg OpenAPIConfig) ([]byte, error) {
	doc, err := buildOpenAPI(a, cfg)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// OpenAPI serves the app's OpenAPI 3.1 spec as JSON at path, with GET, and
// returns the route, in place of the spec an app serves at
// Config.OpenAPIPath. Use it to protect the spec with middleware, or to
// serve several specs.
//
//	app.OpenAPI("/openapi.json", zinc.OpenAPIConfig{Title: "Shop", Version: "1.4.0"})
//
// The spec is built on the first request and sent as pre-encoded bytes after
// that. Routes registered later are picked up on the next request. The spec
// route itself is hidden from the spec. It panics when a security scheme is
// missing a field its type needs, or cfg.Security names a scheme
// cfg.SecuritySchemes doesn't define; a route naming an unknown scheme makes
// the request fail with a 500 and the error.
//
// Hiding the spec isn't access control: protect it, and the API, with auth
// middleware if the API is private.
func (a *App) OpenAPI(path string, cfg OpenAPIConfig, middleware ...HandlerFunc) Route {
	if err := checkOpenAPIConfig(cfg); err != nil {
		panic(err.Error())
	}
	a.spec, a.specPath = nil, ""
	if a.docsPage != nil {
		// The reference page follows the spec to its new path.
		a.renderDocs(path)
	}
	spec := &servedSpec{app: a, cfg: cfg}
	handlers := append(append([]HandlerFunc(nil), middleware...), spec.serve)
	return a.Get(path, handlers...).Hidden()
}

// servedSpec caches the encoded spec with the number of routes it describes.
type servedSpec struct {
	app    *App
	cfg    OpenAPIConfig
	mu     sync.Mutex
	cached atomic.Pointer[encodedSpec]
}

type encodedSpec struct {
	routes  int
	version uint64
	body    []byte
}

func (s *servedSpec) serve(c *Context) error {
	body, err := s.bytes()
	if err != nil {
		return err
	}
	return c.Data(MIMEJSON, body)
}

// bytes returns the encoded spec, rebuilding it when routes were registered,
// or their metadata changed, since it was built. Registration isn't
// concurrent with serving, so reading the counts here is safe.
func (s *servedSpec) bytes() ([]byte, error) {
	routes, version := len(s.app.router.routeInfos), s.app.router.docsVersion
	current := func(c *encodedSpec) bool { return c != nil && c.routes == routes && c.version == version }
	if cached := s.cached.Load(); current(cached) {
		return cached.body, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached := s.cached.Load(); current(cached) {
		return cached.body, nil
	}
	body, err := s.app.OpenAPISpec(s.cfg)
	if err != nil {
		return nil, err
	}
	s.cached.Store(&encodedSpec{routes: routes, version: version, body: body})
	return body, nil
}

// oaMethods are the methods OpenAPI 3.1 has operation fields for, in the
// order the spec lists them.
var oaMethods = []string{
	http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete,
	http.MethodOptions, http.MethodHead, http.MethodPatch, http.MethodTrace,
}

type oaDocument struct {
	OpenAPI      string                               `json:"openapi"`
	Info         oaInfo                               `json:"info"`
	Servers      []OpenAPIServer                      `json:"servers,omitempty"`
	Security     []map[string][]string                `json:"security,omitempty"`
	Tags         []OpenAPITag                         `json:"tags,omitempty"`
	ExternalDocs *OpenAPIExternalDocs                 `json:"externalDocs,omitempty"`
	Paths        orderedMap[orderedMap[*oaOperation]] `json:"paths"`
	Components   oaComponents                         `json:"components"`
}

type oaInfo struct {
	Title          string          `json:"title"`
	Version        string          `json:"version"`
	Description    string          `json:"description,omitempty"`
	TermsOfService string          `json:"termsOfService,omitempty"`
	Contact        *OpenAPIContact `json:"contact,omitempty"`
	License        *OpenAPILicense `json:"license,omitempty"`
}

type oaComponents struct {
	Schemas         map[string]*schema               `json:"schemas,omitempty"`
	SecuritySchemes map[string]OpenAPISecurityScheme `json:"securitySchemes,omitempty"`
}

type oaOperation struct {
	Tags        []string                `json:"tags,omitempty"`
	Summary     string                  `json:"summary,omitempty"`
	Description string                  `json:"description,omitempty"`
	OperationID string                  `json:"operationId,omitempty"`
	Parameters  []oaParameter           `json:"parameters,omitempty"`
	RequestBody *oaRequestBody          `json:"requestBody,omitempty"`
	Responses   orderedMap[*oaResponse] `json:"responses"`
	Deprecated  bool                    `json:"deprecated,omitempty"`
	// Security is a pointer so an explicit empty list, a public route,
	// is written as [] rather than left out.
	Security *[]map[string][]string `json:"security,omitempty"`
}

type oaParameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *schema `json:"schema"`
}

type oaRequestBody struct {
	Required bool                    `json:"required,omitempty"`
	Content  orderedMap[oaMediaType] `json:"content"`
}

type oaResponse struct {
	Description string                  `json:"description"`
	Content     orderedMap[oaMediaType] `json:"content,omitempty"`
}

type oaMediaType struct {
	Schema *schema `json:"schema,omitempty"`
}

// orderedMap is a JSON object that keeps insertion order, so paths follow
// registration order and responses their status order.
type orderedMap[T any] []orderedEntry[T]

type orderedEntry[T any] struct {
	key   string
	value T
}

func (m orderedMap[T]) get(key string) (T, bool) {
	for _, e := range m {
		if e.key == key {
			return e.value, true
		}
	}
	var zero T
	return zero, false
}

func (m *orderedMap[T]) set(key string, value T) {
	for i := range *m {
		if (*m)[i].key == key {
			(*m)[i].value = value
			return
		}
	}
	*m = append(*m, orderedEntry[T]{key, value})
}

// MarshalJSON writes the entries in order.
func (m orderedMap[T]) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, e := range m {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Quote(e.key))
		b.WriteByte(':')
		v, err := json.Marshal(e.value)
		if err != nil {
			return nil, err
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// errorSchemaName is the component for Zinc's error envelope.
const errorSchemaName = "Error"

func buildOpenAPI(a *App, cfg OpenAPIConfig) (*oaDocument, error) {
	title, version := defaultOpenAPIInfo()
	if cfg.Title != "" {
		title = cfg.Title
	}
	if cfg.Version != "" {
		version = cfg.Version
	}
	if err := checkOpenAPIConfig(cfg); err != nil {
		return nil, err
	}
	doc := &oaDocument{
		OpenAPI: "3.1.0",
		Info: oaInfo{
			Title: title, Version: version, Description: cfg.Description,
			TermsOfService: cfg.TermsOfService, Contact: cfg.Contact, License: cfg.License,
		},
		Servers:      cfg.Servers,
		ExternalDocs: cfg.ExternalDocs,
		Tags:         append([]OpenAPITag(nil), cfg.Tags...),
	}
	global, globalScoped, err := securityRequirements(cfg, "OpenAPIConfig.Security", eachAlone(cfg.Security))
	if err != nil {
		return nil, err
	}
	doc.Security = global
	tagged := map[string]bool{}
	for _, tag := range cfg.Tags {
		tagged[tag.Name] = true
	}

	g := newSchemaGen()
	g.types = cfg.Schemas
	if a.defaultErrors {
		// The error envelope is Zinc's: reserve its name before any user type
		// can take it, so a user type named Error gets a qualified name
		// rather than being replaced.
		g.taken[errorSchemaName] = componentKey{t: reflect.TypeFor[errorEnvelopeMarker]()}
	}
	// OpenAPI can't hold two operations at one path and method, or two
	// paths that differ only in parameter names, so either is an error, not
	// a silent replacement.
	shapes := map[string]string{} // path with parameter names removed -> path
	ops := map[string]string{}    // method and OpenAPI path -> route pattern
	// Without a Validator nothing enforces validate tags, so the spec doesn't
	// claim their rules.
	g.validation = a.config.Validator != nil
	usesErrors := false
	table := a.router
	for i, meta := range table.routeInfos {
		if meta.mounted || !slices.Contains(oaMethods, meta.method) {
			continue
		}
		rd := table.routeDocs[uint32(i)]
		if rd == nil {
			rd = &routeDoc{}
		}
		if rd.hidden {
			continue
		}
		// A secured route can be refused, so it documents 401, and 403
		// when it needs scopes.
		reqs, scoped := global, globalScoped
		if rd.securitySet {
			reqs, scoped, err = securityRequirements(cfg, fmt.Sprintf("%s %s", meta.method, meta.path), rd.security)
			if err != nil {
				return nil, err
			}
		}
		var authErrors []int
		if len(reqs) > 0 && !cfg.NoAuthResponses {
			authErrors = append(authErrors, http.StatusUnauthorized)
			if scoped {
				authErrors = append(authErrors, http.StatusForbidden)
			}
		}
		op, errs := buildOperation(g, a, meta, rd, authErrors)
		usesErrors = usesErrors || errs
		if rd.securitySet {
			if reqs == nil {
				reqs = []map[string][]string{}
			}
			op.Security = &reqs
		}
		for _, tag := range op.Tags {
			if !tagged[tag] {
				tagged[tag] = true
				doc.Tags = append(doc.Tags, OpenAPITag{Name: tag})
			}
		}
		path := oaPath(meta.path)
		if other, ok := ops[meta.method+" "+path]; ok {
			return nil, fmt.Errorf("zinc: routes %s %s and %s %s are the same OpenAPI operation, %s %s; hide one with Route.Hidden", meta.method, other, meta.method, meta.path, meta.method, path)
		}
		ops[meta.method+" "+path] = meta.path
		shape := pathShape(path)
		if other, ok := shapes[shape]; ok && other != path {
			return nil, fmt.Errorf("zinc: paths %s and %s differ only in parameter names, which OpenAPI doesn't allow; give the parameters the same names", other, path)
		}
		shapes[shape] = path
		item, _ := doc.Paths.get(path)
		item.set(strings.ToLower(meta.method), op)
		slices.SortStableFunc(item, func(x, y orderedEntry[*oaOperation]) int {
			return slices.Index(oaMethods, strings.ToUpper(x.key)) - slices.Index(oaMethods, strings.ToUpper(y.key))
		})
		doc.Paths.set(path, item)
	}
	if doc.Paths == nil {
		doc.Paths = orderedMap[orderedMap[*oaOperation]]{}
	}
	if usesErrors {
		g.components[errorSchemaName] = errorEnvelopeSchema()
	}
	if len(g.components) > 0 {
		doc.Components.Schemas = g.components
	}
	if len(cfg.SecuritySchemes) > 0 {
		schemes, err := checkSecuritySchemes(cfg.SecuritySchemes)
		if err != nil {
			return nil, err
		}
		doc.Components.SecuritySchemes = schemes
	}
	return doc, nil
}

// buildOperation describes one route, and reports whether it can answer with
// Zinc's error envelope.
func buildOperation(g *schemaGen, a *App, meta routeMeta, rd *routeDoc, authErrors []int) (*oaOperation, bool) {
	op := &oaOperation{
		Tags:        rd.tags,
		Summary:     rd.summary,
		Description: rd.description,
		OperationID: meta.name,
		Deprecated:  rd.deprecated,
	}

	// Parameters: every path parameter, typed from the input when it binds
	// one, then the input's query and header fields.
	in := rd.in
	var plan *bindingPlan
	if in != nil && base(in).Kind() == reflect.Struct {
		in = base(in)
		plan = bindingPlanFor(in)
	}
	// A parameter is absent or a value, never null, so a pointer field is
	// described by its element.
	paramSchema := func(f bindingField) (*schema, bool) {
		sf := f.structField(in)
		s := g.inputSchemaFor(base(sf.Type))
		if s.ref != "" {
			s = &schema{ref: s.ref} // annotations sit beside a shared $ref
		}
		required := g.applyFieldTags(s, jsonField{typ: sf.Type, tag: sf.Tag})
		s.def = defaultValue(sf.Type, f.def)
		return s, required
	}
	for _, name := range meta.params {
		p := oaParameter{Name: name, In: "path", Required: true, Schema: &schema{typ: []string{"string"}}}
		if strings.Contains(meta.path, "{"+name+"...}") {
			p.Description = "The rest of the path, which may contain slashes."
		}
		if plan != nil {
			for _, f := range plan.pathFields {
				if f.name == name {
					p.Schema, _ = paramSchema(f)
				}
			}
		}
		op.Parameters = append(op.Parameters, p)
	}
	hasInput := len(meta.params) > 0 && plan != nil && len(plan.pathFields) > 0
	if plan != nil {
		for _, f := range plan.queryFields {
			s, required := paramSchema(f)
			op.Parameters = append(op.Parameters, oaParameter{Name: f.name, In: "query", Required: required, Schema: s})
			hasInput = true
		}
		for _, f := range plan.headerFields {
			s, required := paramSchema(f)
			op.Parameters = append(op.Parameters, oaParameter{Name: f.headerName, In: "header", Required: required, Schema: s})
			hasInput = true
		}
		for _, f := range plan.cookieFields {
			s, required := paramSchema(f)
			op.Parameters = append(op.Parameters, oaParameter{Name: f.name, In: "cookie", Required: required, Schema: s})
			hasInput = true
		}
	}

	// The body. GET and HEAD requests have none in practice, so they're
	// never documented with one.
	if rd.in != nil && meta.method != http.MethodGet && meta.method != http.MethodHead {
		if body := buildRequestBody(g, rd.in, plan); body != nil {
			op.RequestBody = body
			hasInput = true
		}
	}

	// Responses: success, then binding and validation failures, then the
	// ones the route declares. A declared status replaces a derived one.
	status := int(meta.status)
	out := rd.out
	noContent := out == reflect.TypeFor[NoContent]()
	if status == 0 {
		status = http.StatusOK
		if noContent {
			status = http.StatusNoContent
		}
	}
	success := &oaResponse{Description: http.StatusText(status)}
	switch {
	case out == nil:
		// A plain handler may write anything.
		success.Content = anyContent()
	case !noContent && status != http.StatusNoContent:
		success.Content = orderedMap[oaMediaType]{{"application/json", oaMediaType{Schema: g.schemaFor(out)}}}
	}
	responses := map[int]*oaResponse{}
	// Without an output type or a success status, Zinc can't know what the
	// handler answers, so it documents a default response rather than guess.
	var fallback *oaResponse
	if out == nil && meta.status == 0 && !declaresSuccess(rd) {
		fallback = &oaResponse{
			Description: "The handler's response. Route.Output or Route.Response describes it.",
			Content:     anyContent(),
		}
	} else if !declaresSuccess(rd) || out != nil || meta.status != 0 {
		responses[status] = success
	}
	// Any route can fail, so every one documents a 500. The error body is
	// described only when Zinc's default error handler writes it; a custom
	// ErrorHandler may write anything.
	errorBody := a.defaultErrors
	responses[http.StatusInternalServerError] = errorResponse(http.StatusInternalServerError, errorBody)
	if hasInput {
		responses[http.StatusBadRequest] = errorResponse(http.StatusBadRequest, errorBody)
		if a.config.Validator != nil {
			responses[http.StatusUnprocessableEntity] = errorResponse(http.StatusUnprocessableEntity, errorBody)
		}
	}
	usesErrors := errorBody
	for _, status := range authErrors {
		responses[status] = errorResponse(status, errorBody)
	}
	for _, status := range rd.errors {
		responses[status] = errorResponse(status, errorBody)
	}
	for _, r := range rd.responses {
		resp := &oaResponse{Description: http.StatusText(r.status)}
		if r.typ != nil {
			resp.Content = orderedMap[oaMediaType]{{"application/json", oaMediaType{Schema: g.schemaFor(r.typ)}}}
		}
		responses[r.status] = resp
	}
	codes := make([]int, 0, len(responses))
	for code := range responses {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	for _, code := range codes {
		op.Responses.set(strconv.Itoa(code), responses[code])
	}
	if fallback != nil {
		op.Responses.set("default", fallback)
	}
	return op, usesErrors
}

// declaresSuccess reports whether the route documents a 2xx with
// Route.Response.
func declaresSuccess(rd *routeDoc) bool {
	for _, r := range rd.responses {
		if r.status >= 200 && r.status < 300 {
			return true
		}
	}
	return false
}

// anyContent is a body of any media type and shape.
func anyContent() orderedMap[oaMediaType] {
	return orderedMap[oaMediaType]{{"*/*", oaMediaType{}}}
}

// buildRequestBody describes the body an input type accepts: JSON for its
// body fields, and a form for its form fields.
func buildRequestBody(g *schemaGen, in reflect.Type, plan *bindingPlan) *oaRequestBody {
	body := &oaRequestBody{}
	if base(in).Kind() != reflect.Struct {
		body.Content.set("application/json", oaMediaType{Schema: g.inputSchemaFor(in)})
		return body
	}
	st := base(in)
	var jsonProps, required bool
	for _, f := range jsonFields(st) {
		if f.param {
			continue
		}
		// A JSON body binds a form field too, but a struct of form fields
		// alone is a form, so JSON is documented only when a field has a
		// json tag or no form tag.
		_, hasJSON := f.tag.Lookup("json")
		_, hasForm := f.tag.Lookup("form")
		if hasJSON || !hasForm {
			jsonProps = true
			if g.validation && strings.Contains(","+f.tag.Get("validate")+",", ",required,") {
				required = true
			}
		}
	}
	if jsonProps {
		body.Content.set("application/json", oaMediaType{Schema: g.bodySchemaFor(st)})
	}
	if plan != nil && (len(plan.formFields) > 0 || len(plan.multipartFileFields) > 0) {
		form := &schema{typ: []string{"object"}}
		for _, f := range plan.formFields {
			sf := f.structField(st)
			s := g.inputSchemaFor(sf.Type)
			if s.ref != "" {
				s = &schema{ref: s.ref}
			}
			s.def = defaultValue(sf.Type, f.def)
			if g.applyFieldTags(s, jsonField{typ: sf.Type, tag: sf.Tag}) {
				form.required = append(form.required, f.name)
				required = true
			}
			form.properties = append(form.properties, property{name: f.name, schema: s})
		}
		for _, f := range plan.multipartFileFields {
			file := &schema{typ: []string{"string"}, contentMediaType: "application/octet-stream"}
			if base(f.structField(st).Type).Kind() == reflect.Slice {
				file = &schema{typ: []string{"array"}, items: file}
			}
			form.properties = append(form.properties, property{name: f.name, schema: file})
		}
		mediaType := "application/x-www-form-urlencoded"
		if len(plan.multipartFileFields) > 0 {
			mediaType = "multipart/form-data"
		}
		body.Content.set(mediaType, oaMediaType{Schema: form})
	}
	if len(body.Content) == 0 {
		return nil
	}
	body.Required = required
	return body
}

// defaultValue writes a default tag's inputs as a JSON value of the field's
// type: a list for a slice, a string for a type that parses text itself.
func defaultValue(t reflect.Type, def []string) any {
	if def == nil {
		return nil
	}
	t = base(t)
	one := func(text string, t reflect.Type) any {
		if v, ok := parseScalar(text, t, false); ok {
			return v
		}
		return text
	}
	if t.Kind() == reflect.Slice && compileFieldSetter(t).usesAllValues() {
		out := make([]any, len(def))
		for i, text := range def {
			out[i] = one(text, base(t.Elem()))
		}
		return out
	}
	return one(def[0], t)
}

// errorResponse describes an error status, with Zinc's error envelope as its
// body when withBody is set.
func errorResponse(status int, withBody bool) *oaResponse {
	if !withBody {
		return &oaResponse{Description: http.StatusText(status)}
	}
	return &oaResponse{
		Description: http.StatusText(status),
		Content: orderedMap[oaMediaType]{{"application/json", oaMediaType{
			Schema: &schema{ref: "#/components/schemas/" + errorSchemaName},
		}}},
	}
}

// errorEnvelopeSchema describes the body DefaultErrorHandler writes.
func errorEnvelopeSchema() *schema {
	return &schema{
		typ:         []string{"object"},
		description: "The error body Zinc's default error handler writes.",
		properties: []property{{"error", &schema{
			typ: []string{"object"},
			properties: []property{
				{"status", &schema{typ: []string{"integer"}, format: "int32", description: "The HTTP status."}},
				{"message", &schema{typ: []string{"string"}}},
				{"fields", &schema{typ: []string{"object"}, description: "Invalid fields, by name, and what's wrong with each.", additionalProperties: &schema{typ: []string{"string"}}}},
				{"details", &schema{typ: []string{"object"}, description: "Details added with HTTPError.Details."}},
			},
			required: []string{"status", "message"},
		}}},
		required: []string{"error"},
	}
}

// errorEnvelopeMarker stands for Zinc's error envelope in the component
// names a spec reserves.
type errorEnvelopeMarker struct{}

// pathShape is an OpenAPI path with its parameter names removed:
// /pets/{id} and /pets/{name} share the shape /pets/{}.
func pathShape(path string) string {
	var b strings.Builder
	for {
		open := strings.IndexByte(path, '{')
		if open < 0 {
			b.WriteString(path)
			return b.String()
		}
		end := strings.IndexByte(path[open:], '}')
		if end < 0 {
			b.WriteString(path)
			return b.String()
		}
		b.WriteString(path[:open+1])
		b.WriteByte('}')
		path = path[open+end+1:]
	}
}

// oaPath writes a route pattern as an OpenAPI path: a wildcard {rest...}
// becomes {rest}.
func oaPath(pattern string) string {
	return strings.ReplaceAll(pattern, "...}", "}")
}

// eachAlone turns scheme names that each suffice into requirements.
func eachAlone(names []string) [][]string {
	out := make([][]string, len(names))
	for i, name := range names {
		out[i] = []string{name}
	}
	return out
}

// securityRequirements writes requirements, each a list of schemes that
// must all pass, as OpenAPI security requirement objects. A name that isn't
// a scheme but starts with one and a colon carries a scope:
// "oauth:pets:read" is scheme oauth with scope pets:read. It reports whether
// any requirement has a scope.
func securityRequirements(cfg OpenAPIConfig, where string, reqs [][]string) ([]map[string][]string, bool, error) {
	if len(reqs) == 0 {
		return nil, false, nil
	}
	scoped := false
	out := make([]map[string][]string, 0, len(reqs))
	for _, req := range reqs {
		m := map[string][]string{}
		for _, name := range req {
			scheme, scope := name, ""
			if _, ok := cfg.SecuritySchemes[name]; !ok {
				if before, after, found := strings.Cut(name, ":"); found {
					scheme, scope = before, after
				}
			}
			if _, ok := cfg.SecuritySchemes[scheme]; !ok {
				return nil, false, fmt.Errorf("zinc: %s names security scheme %q, which OpenAPIConfig.SecuritySchemes doesn't define", where, name)
			}
			scopes := m[scheme]
			if scopes == nil {
				scopes = []string{}
			}
			if scope != "" && !slices.Contains(scopes, scope) {
				scopes = append(scopes, scope)
				scoped = true
			}
			m[scheme] = scopes
		}
		out = append(out, m)
	}
	return out, scoped, nil
}

// checkOpenAPIConfig reports a config the spec can't be valid with.
func checkOpenAPIConfig(cfg OpenAPIConfig) error {
	if _, err := checkSecuritySchemes(cfg.SecuritySchemes); err != nil {
		return err
	}
	if _, _, err := securityRequirements(cfg, "OpenAPIConfig.Security", eachAlone(cfg.Security)); err != nil {
		return err
	}
	if l := cfg.License; l != nil && (l.Name == "" || (l.Identifier != "" && l.URL != "")) {
		return errors.New("zinc: OpenAPIConfig.License needs Name, and Identifier or URL but not both")
	}
	if cfg.ExternalDocs != nil && cfg.ExternalDocs.URL == "" {
		return errors.New("zinc: OpenAPIConfig.ExternalDocs needs URL")
	}
	for _, tag := range cfg.Tags {
		if tag.Name == "" {
			return errors.New("zinc: every OpenAPIConfig.Tags entry needs Name")
		}
		if tag.ExternalDocs != nil && tag.ExternalDocs.URL == "" {
			return fmt.Errorf("zinc: tag %q: ExternalDocs needs URL", tag.Name)
		}
	}
	return nil
}

// defaultOpenAPIInfo takes a title and version from the main module.
func defaultOpenAPIInfo() (title, version string) {
	title, version = "API", "0.0.0"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return title, version
	}
	if path := info.Main.Path; path != "" {
		title = path[strings.LastIndexByte(path, '/')+1:]
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		version = strings.TrimPrefix(v, "v")
	}
	return title, version
}

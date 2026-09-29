// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"encoding/json"
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
	// Servers lists base URLs the API is served from.
	Servers []OpenAPIServer
	// SecuritySchemes defines the schemes Route.Security, Group.Security and
	// Security refer to, by name.
	SecuritySchemes map[string]OpenAPISecurityScheme
	// Security names the schemes that protect every route that doesn't set
	// its own with Route.Security or Group.Security.
	Security []string
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
// returns the route. It's off unless you call it, so an app never exposes its
// API's shape by accident.
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
	if _, err := checkSecuritySchemes(cfg.SecuritySchemes); err != nil {
		panic(err.Error())
	}
	for _, name := range cfg.Security {
		if _, ok := cfg.SecuritySchemes[name]; !ok {
			panic(fmt.Sprintf("zinc: OpenAPIConfig.Security names security scheme %q, which OpenAPIConfig.SecuritySchemes doesn't define", name))
		}
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
	routes int
	body   []byte
}

func (s *servedSpec) serve(c *Context) error {
	body, err := s.bytes()
	if err != nil {
		return err
	}
	return c.Data(MIMEJSON, body)
}

// bytes returns the encoded spec, rebuilding it when routes were registered
// since it was built. Registration isn't concurrent with serving, so reading
// the route count here is safe.
func (s *servedSpec) bytes() ([]byte, error) {
	routes := len(s.app.router.routeInfos)
	if cached := s.cached.Load(); cached != nil && cached.routes == routes {
		return cached.body, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached := s.cached.Load(); cached != nil && cached.routes == routes {
		return cached.body, nil
	}
	body, err := s.app.OpenAPISpec(s.cfg)
	if err != nil {
		return nil, err
	}
	s.cached.Store(&encodedSpec{routes: routes, body: body})
	return body, nil
}

// oaMethods are the methods OpenAPI 3.1 has operation fields for, in the
// order the spec lists them.
var oaMethods = []string{
	http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete,
	http.MethodOptions, http.MethodHead, http.MethodPatch, http.MethodTrace,
}

type oaDocument struct {
	OpenAPI    string                               `json:"openapi"`
	Info       oaInfo                               `json:"info"`
	Servers    []OpenAPIServer                      `json:"servers,omitempty"`
	Security   []map[string][]string                `json:"security,omitempty"`
	Paths      orderedMap[orderedMap[*oaOperation]] `json:"paths"`
	Components oaComponents                         `json:"components"`
}

type oaInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
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
	doc := &oaDocument{
		OpenAPI: "3.1.0",
		Info:    oaInfo{Title: title, Version: version, Description: cfg.Description},
		Servers: cfg.Servers,
	}
	checkScheme := func(where string, names []string) error {
		for _, name := range names {
			if _, ok := cfg.SecuritySchemes[name]; !ok {
				return fmt.Errorf("zinc: %s names security scheme %q, which OpenAPIConfig.SecuritySchemes doesn't define", where, name)
			}
		}
		return nil
	}
	if err := checkScheme("OpenAPIConfig.Security", cfg.Security); err != nil {
		return nil, err
	}
	doc.Security = securityRequirements(cfg.Security)

	g := newSchemaGen()
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
		op, errs := buildOperation(g, a, meta, rd)
		usesErrors = usesErrors || errs
		if rd.securitySet {
			if err := checkScheme(fmt.Sprintf("%s %s", meta.method, meta.path), rd.security); err != nil {
				return nil, err
			}
			reqs := securityRequirements(rd.security)
			if reqs == nil {
				reqs = []map[string][]string{}
			}
			op.Security = &reqs
		}
		path := oaPath(meta.path)
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
func buildOperation(g *schemaGen, a *App, meta routeMeta, rd *routeDoc) (*oaOperation, bool) {
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
	var fields map[int]reflect.StructField
	if in != nil && base(in).Kind() == reflect.Struct {
		in = base(in)
		plan = bindingPlanFor(in)
		fields = map[int]reflect.StructField{}
		for i := range in.NumField() {
			fields[i] = in.Field(i)
		}
	}
	// A parameter is absent or a value, never null, so a pointer field is
	// described by its element.
	paramSchema := func(f bindingField) (*schema, bool) {
		sf := fields[f.index]
		s := g.inputSchemaFor(base(sf.Type))
		required := g.applyFieldTags(s, jsonField{typ: sf.Type, tag: sf.Tag})
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
			sf := st.Field(f.index)
			s := g.inputSchemaFor(sf.Type)
			if g.applyFieldTags(s, jsonField{typ: sf.Type, tag: sf.Tag}) {
				form.required = append(form.required, f.name)
				required = true
			}
			form.properties = append(form.properties, property{name: f.name, schema: s})
		}
		for _, f := range plan.multipartFileFields {
			file := &schema{typ: []string{"string"}, contentMediaType: "application/octet-stream"}
			if base(st.Field(f.index).Type).Kind() == reflect.Slice {
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

// oaPath writes a route pattern as an OpenAPI path: a wildcard {rest...}
// becomes {rest}.
func oaPath(pattern string) string {
	return strings.ReplaceAll(pattern, "...}", "}")
}

func securityRequirements(names []string) []map[string][]string {
	if len(names) == 0 {
		return nil
	}
	out := make([]map[string][]string, len(names))
	for i, name := range names {
		out[i] = map[string][]string{name: {}}
	}
	return out
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

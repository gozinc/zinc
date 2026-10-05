// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// routeDoc is a route's OpenAPI metadata. Registration and Route methods
// write it; only the spec reads it.
type routeDoc struct {
	// in and out are the request and success-response types: from Typed, or
	// from Route.Input and Route.Output. typed marks the Typed case.
	in, out     reflect.Type
	typed       bool
	summary     string
	description string
	tags        []string
	deprecated  bool
	hidden      bool
	responses   []docResponse
	// errors are statuses the route answers through the error handler.
	errors []int
	// security lists requirements, each the schemes that must all pass;
	// any one requirement is enough. securitySet tells an explicit empty
	// list (a public route) from no setting at all.
	security    [][]string
	securitySet bool
	// produces lists media types by status, from Route.Produces; consumes
	// lists the request body's, from Route.Consumes.
	produces map[int][]string
	consumes []string
	// examples are named examples, from Route.Example and
	// Route.RequestExample, in the order given.
	examples []docExample
	// operationHooks edit the route's operation, from Route.Operation.
	operationHooks []func(op map[string]any)
	// middleware describes what the route's middleware adds, from
	// Group.Document and Route.Document.
	middleware []MiddlewareDoc
}

// MiddlewareDoc describes, for the OpenAPI spec, what a middleware adds to
// the routes it runs on: the credentials and request headers it reads and
// the errors it can answer. A middleware is a plain function, so Zinc can't see this for
// itself. Pass it to App.Document, Group.Document or Route.Document beside
// the middleware:
//
//	api.Use(csrf.New())
//	api.Document(csrf.Doc())
type MiddlewareDoc struct {
	// Methods limits the description to requests with these methods, such
	// as the unsafe ones CSRF protection checks. Empty means every method.
	Methods []string
	// Security holds credentials the middleware checks on every request it
	// covers, such as a CSRF token, by scheme name. Each is added to the
	// spec's security schemes and required together with the route's own
	// security, so a generated client sets it once rather than on every
	// call. A scheme of the same name in OpenAPIConfig.SecuritySchemes must
	// be the same. Unlike a route's security, it adds no 401 or 403; list
	// what the middleware answers in Errors.
	Security map[string]OpenAPISecurityScheme
	// Headers are request headers the middleware reads, other than
	// credentials: each is a parameter callers pass.
	Headers []HeaderDoc
	// Errors are statuses it answers by returning an error, described with
	// the body the error handler writes, as Route.Errors does.
	Errors []int
}

// HeaderDoc is a request header a middleware reads.
type HeaderDoc struct {
	// Name is the header's name, spelled as the spec should show it.
	Name        string
	Description string
	// Required says requests the middleware runs on must send it.
	Required bool
}

// mustMiddlewareDocs checks and copies docs passed to a Document method.
func mustMiddlewareDocs(method string, docs []MiddlewareDoc) []MiddlewareDoc {
	out := make([]MiddlewareDoc, len(docs))
	for i, d := range docs {
		for _, status := range d.Errors {
			if status < 400 || status > 599 {
				panic(fmt.Sprintf("zinc: %s error status %d is not an error status", method, status))
			}
		}
		for _, h := range d.Headers {
			if strings.TrimSpace(h.Name) == "" {
				panic("zinc: " + method + " header needs a Name")
			}
		}
		for name := range d.Security {
			if strings.TrimSpace(name) == "" {
				panic("zinc: " + method + " security scheme needs a name")
			}
		}
		methods := make([]string, len(d.Methods))
		for j, m := range d.Methods {
			methods[j] = strings.ToUpper(m)
		}
		out[i] = MiddlewareDoc{Methods: methods, Security: maps.Clone(d.Security), Headers: slices.Clone(d.Headers), Errors: slices.Clone(d.Errors)}
	}
	return out
}

// appliesTo reports whether the description covers requests with method.
func (d MiddlewareDoc) appliesTo(method string) bool {
	return len(d.Methods) == 0 || slices.Contains(d.Methods, method)
}

// middlewareFor lists the descriptions that cover a route's requests with
// method, the app's first.
func middlewareFor(a *App, rd *routeDoc, method string) []MiddlewareDoc {
	var out []MiddlewareDoc
	for _, docs := range [][]MiddlewareDoc{a.middlewareDocs, rd.middleware} {
		for _, d := range docs {
			if d.appliesTo(method) {
				out = append(out, d)
			}
		}
	}
	return out
}

// Document adds descriptions of the route's own middleware, the handlers
// before its last, to the spec.
func (r Route) Document(docs ...MiddlewareDoc) Route {
	doc := r.doc("Document")
	doc.middleware = append(doc.middleware, mustMiddlewareDocs("Document", docs)...)
	return r
}

// docExample is a named example of a response, or of the request body when
// status is 0.
type docExample struct {
	status int
	name   string
	value  any
}

// docResponse is an extra response declared with Route.Response. A nil typ
// means the response has no body.
type docResponse struct {
	status int
	typ    reflect.Type
}

// doc returns the route's metadata for a change, creating it on first use.
func (r *routeTable) doc(index uint32) *routeDoc {
	r.docsVersion++
	if r.routeDocs == nil {
		r.routeDocs = make(map[uint32]*routeDoc)
	}
	doc := r.routeDocs[index]
	if doc == nil {
		doc = &routeDoc{}
		r.routeDocs[index] = doc
	}
	return doc
}

// doc returns the route's metadata for a Route method named method.
func (r Route) doc(method string) *routeDoc {
	if r.table == nil {
		panic("zinc: " + method + " on a route that was not registered")
	}
	return r.table.doc(r.index)
}

// Summary sets the route's one-line summary in the OpenAPI spec.
func (r Route) Summary(summary string) Route {
	r.doc("Summary").summary = summary
	return r
}

// Description sets the route's longer description in the OpenAPI spec. It
// may use Markdown.
func (r Route) Description(description string) Route {
	r.doc("Description").description = description
	return r
}

// Tags adds tags that group the route in the OpenAPI spec, after any from
// its group.
func (r Route) Tags(tags ...string) Route {
	doc := r.doc("Tags")
	doc.tags = appendNew(doc.tags, tags)
	return r
}

// Deprecated marks the route as deprecated in the OpenAPI spec. The route
// still serves requests.
func (r Route) Deprecated() Route {
	r.doc("Deprecated").deprecated = true
	return r
}

// Hidden leaves the route out of the OpenAPI spec. The route still serves
// requests.
func (r Route) Hidden() Route {
	r.doc("Hidden").hidden = true
	return r
}

// Input declares the request type of a route whose handler isn't Typed, so
// the OpenAPI spec can describe its parameters and body. Pass a value of the
// type, such as CreateUser{}. It panics on a Typed route, whose input is
// already known.
func (r Route) Input(v any) Route {
	doc := r.doc("Input")
	doc.in = declaredType("Input", doc, v)
	return r
}

// Output declares the success-response type of a route whose handler isn't
// Typed, such as User{}. It panics on a Typed route, whose output is already
// known.
func (r Route) Output(v any) Route {
	doc := r.doc("Output")
	out := declaredType("Output", doc, v)
	r.table.checkRedirectStatus(r.index, out, int(r.table.routeInfos[r.index].status))
	doc.out = out
	return r
}

// Response declares another response the route can send, such as a 409 with
// a ConflictBody{}. A nil value means the response has no body. Declaring the
// same status again replaces it.
func (r Route) Response(status int, v any) Route {
	if status < 100 || status > 599 {
		panic(fmt.Sprintf("zinc: Response status %d is not an HTTP status", status))
	}
	doc := r.doc("Response")
	response := docResponse{status: status, typ: reflect.TypeOf(v)}
	for i := range doc.responses {
		if doc.responses[i].status == status {
			doc.responses[i] = response
			return r
		}
	}
	doc.responses = append(doc.responses, response)
	return r
}

// Produces declares the media types the route sends with status, such as
// .Produces(200, "text/csv"). Use it where Zinc can't tell from the route: a
// plain handler, or a Bytes, File or Stream output, whose media type is
// chosen when the handler runs. It applies to the success status or a status
// declared with Response. Calling it again for a status replaces its types.
func (r Route) Produces(status int, mediaTypes ...string) Route {
	if status < 100 || status > 599 {
		panic(fmt.Sprintf("zinc: Produces status %d is not an HTTP status", status))
	}
	mustMediaTypes("Produces", mediaTypes)
	doc := r.doc("Produces")
	if doc.produces == nil {
		doc.produces = map[int][]string{}
	}
	doc.produces[status] = slices.Clone(mediaTypes)
	return r
}

// Example adds a named example of the response sent with status, shown in
// the spec beside its schema:
//
//	app.Get("/pets/{id}", zinc.Typed(getPet)).
//		Example(200, "a cat", Pet{ID: "7", Name: "Tom"}).
//		Errors(404).
//		Example(404, "unknown id", zinc.NewError(404, "pet not found"))
//
// value is what the route sends for it, as a Go value: the output type for
// the success status, the type given to Response for a declared status, or
// an *HTTPError for an error status, shown as the app's ErrorHandler writes
// it. The spec fails to build when value doesn't match the response it's
// for. An example with the same status and name replaces the earlier one.
func (r Route) Example(status int, name string, value any) Route {
	if status < 100 || status > 599 {
		panic(fmt.Sprintf("zinc: Example status %d is not an HTTP status", status))
	}
	r.doc("Example").addExample(docExample{status: status, name: mustExampleName("Example", name), value: value})
	return r
}

// RequestExample adds a named example of the request body, a value of the
// route's input type. Fields bound from the path, query, headers or cookies
// are left out, as the body doesn't carry them, and so are nil fields.
func (r Route) RequestExample(name string, value any) Route {
	r.doc("RequestExample").addExample(docExample{name: mustExampleName("RequestExample", name), value: value})
	return r
}

func (d *routeDoc) addExample(ex docExample) {
	for i := range d.examples {
		if d.examples[i].status == ex.status && d.examples[i].name == ex.name {
			d.examples[i] = ex
			return
		}
	}
	d.examples = append(d.examples, ex)
}

func mustExampleName(method, name string) string {
	if strings.TrimSpace(name) == "" {
		panic("zinc: " + method + " needs a name, such as \"a cat\"")
	}
	return name
}

// Operation adds a hook that edits the route's operation in the spec, as
// decoded JSON, for what no Route method sets, such as an extension:
//
//	app.Get("/pets", listPets).Operation(func(op map[string]any) {
//		op["x-rate-limit"] = 100
//	})
//
// Hooks run in the order added, before OpenAPIConfig.Mutate, and the spec is
// checked again after them.
func (r Route) Operation(hook func(op map[string]any)) Route {
	if hook == nil {
		panic("zinc: Operation hook is nil")
	}
	doc := r.doc("Operation")
	doc.operationHooks = append(doc.operationHooks, hook)
	return r
}

// Consumes declares the media types the route accepts as a request body,
// such as .Consumes("text/csv"), in place of the ones Zinc infers from the
// input type.
func (r Route) Consumes(mediaTypes ...string) Route {
	mustMediaTypes("Consumes", mediaTypes)
	doc := r.doc("Consumes")
	doc.consumes = slices.Clone(mediaTypes)
	return r
}

func mustMediaTypes(method string, mediaTypes []string) {
	if len(mediaTypes) == 0 {
		panic("zinc: " + method + " needs at least one media type, such as \"text/csv\"")
	}
	for _, mt := range mediaTypes {
		if i := strings.IndexByte(mt, '/'); i <= 0 || i == len(mt)-1 || strings.ContainsAny(mt, " ;,") {
			panic(fmt.Sprintf("zinc: %s media type %q isn't a type/subtype, such as \"text/csv\"", method, mt))
		}
	}
}

// Errors declares error statuses the route can answer by returning an error,
// such as zinc.NewError(http.StatusConflict, ...). The spec describes each
// with the body the error handler writes: Zinc's error envelope with the
// default handler, or the status alone with a custom one. Use Response for a
// status the handler writes itself.
func (r Route) Errors(statuses ...int) Route {
	doc := r.doc("Errors")
	for _, status := range statuses {
		if status < 400 || status > 599 {
			panic(fmt.Sprintf("zinc: Errors status %d is not an error status", status))
		}
		if !slices.Contains(doc.errors, status) {
			doc.errors = append(doc.errors, status)
		}
	}
	return r
}

// Security names the security schemes that protect the route, replacing its
// group's. Any one of them is enough; SecurityAll needs them all. A name can
// carry a scope after a colon, such as "oauth:pets:read". With no names, the
// route is marked public.
func (r Route) Security(schemes ...string) Route {
	doc := r.doc("Security")
	doc.security = eachAlone(schemes)
	doc.securitySet = true
	return r
}

// SecurityAll is Security for schemes that must all pass together, such as
// an API key and a bearer token, replacing the group's.
func (r Route) SecurityAll(schemes ...string) Route {
	doc := r.doc("SecurityAll")
	doc.security = allTogether(schemes)
	doc.securitySet = true
	return r
}

// allTogether is one requirement listing every scheme, or none for none.
func allTogether(schemes []string) [][]string {
	if len(schemes) == 0 {
		return [][]string{}
	}
	return [][]string{append([]string(nil), schemes...)}
}

// declaredType validates a value passed to Input or Output.
func declaredType(method string, doc *routeDoc, v any) reflect.Type {
	if doc.typed {
		panic("zinc: " + method + " on a Typed route; its types come from the handler")
	}
	if v == nil {
		panic("zinc: " + method + " needs a value of the type, such as User{}")
	}
	return reflect.TypeOf(v)
}

// appendNew appends the values not already in list, keeping their order.
func appendNew(list, values []string) []string {
	for _, v := range values {
		if v != "" && !slices.Contains(list, v) {
			list = append(list, v)
		}
	}
	return list
}

// groupDocs holds the OpenAPI defaults a group gives its routes.
type groupDocs struct {
	tags        []string
	security    [][]string
	securitySet bool
	hidden      bool
	middleware  []MiddlewareDoc
}

// Tags adds tags to every route registered in the group from now on, and to
// its child groups. Like Use, it panics once the group has routes or child
// groups, so the order of calls can't silently change the spec.
func (g *Group) Tags(tags ...string) *Group {
	g.mustBeOpen("Tags")
	g.docs.tags = appendNew(g.docs.tags, tags)
	return g
}

// Security names the security schemes that protect every route registered in
// the group from now on, and its child groups. A route can replace them with
// Route.Security. Like Use, it panics once the group has routes.
func (g *Group) Security(schemes ...string) *Group {
	g.mustBeOpen("Security")
	g.docs.security = eachAlone(schemes)
	g.docs.securitySet = true
	return g
}

// SecurityAll is Security for schemes that must all pass together.
func (g *Group) SecurityAll(schemes ...string) *Group {
	g.mustBeOpen("SecurityAll")
	g.docs.security = allTogether(schemes)
	g.docs.securitySet = true
	return g
}

// Hidden leaves every route registered in the group from now on, and its
// child groups' routes, out of the spec. Like Use, it panics once the group
// has routes.
func (g *Group) Hidden() *Group {
	g.mustBeOpen("Hidden")
	g.docs.hidden = true
	return g
}

// Document adds descriptions of the group's middleware to every route
// registered in the group from now on, and its child groups' routes. Like
// Use, it panics once the group has routes.
func (g *Group) Document(docs ...MiddlewareDoc) *Group {
	g.mustBeOpen("Document")
	g.docs.middleware = append(g.docs.middleware, mustMiddlewareDocs("Document", docs)...)
	return g
}

// mustBeOpen panics when the group has already captured its settings.
func (g *Group) mustBeOpen(method string) {
	if g.sealedBy == "" {
		return
	}
	prefix := g.prefix
	if prefix == "" {
		prefix = "/"
	}
	panic(fmt.Sprintf("zinc: %s on group %q after %s; set it before the group's routes and child groups", method, prefix, g.sealedBy))
}

// apply copies the group's defaults onto a route it registered.
func (d groupDocs) apply(r Route) {
	if len(d.tags) == 0 && !d.securitySet && !d.hidden && len(d.middleware) == 0 {
		return
	}
	doc := r.table.doc(r.index)
	doc.tags = appendNew(append([]string(nil), d.tags...), doc.tags)
	if d.securitySet && !doc.securitySet {
		doc.security = cloneRequirements(d.security)
		doc.securitySet = true
	}
	doc.hidden = doc.hidden || d.hidden
	doc.middleware = append(slices.Clone(d.middleware), doc.middleware...)
}

// inherit copies the defaults into a child group.
func (d groupDocs) inherit() groupDocs {
	return groupDocs{
		tags:        append([]string(nil), d.tags...),
		security:    cloneRequirements(d.security),
		securitySet: d.securitySet,
		hidden:      d.hidden,
		middleware:  slices.Clone(d.middleware),
	}
}

func cloneRequirements(reqs [][]string) [][]string {
	if reqs == nil {
		return nil
	}
	out := make([][]string, len(reqs))
	for i, req := range reqs {
		out[i] = append([]string(nil), req...)
	}
	return out
}

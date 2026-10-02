// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
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
	doc.out = declaredType("Output", doc, v)
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
	if len(d.tags) == 0 && !d.securitySet && !d.hidden {
		return
	}
	doc := r.table.doc(r.index)
	doc.tags = appendNew(append([]string(nil), d.tags...), doc.tags)
	if d.securitySet && !doc.securitySet {
		doc.security = cloneRequirements(d.security)
		doc.securitySet = true
	}
	doc.hidden = doc.hidden || d.hidden
}

// inherit copies the defaults into a child group.
func (d groupDocs) inherit() groupDocs {
	return groupDocs{
		tags:        append([]string(nil), d.tags...),
		security:    cloneRequirements(d.security),
		securitySet: d.securitySet,
		hidden:      d.hidden,
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

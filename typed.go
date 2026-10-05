// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"reflect"
	"sync"
)

// NoContent is the output type of a typed handler that sends no body. The
// route answers 204 No Content, or the status declared with Route.Status,
// even 200.
type NoContent struct{}

// Typed adapts a function whose signature is the request contract into a
// handler. The input is bound from the request, validated, and passed in; the
// output is written as JSON:
//
//	type CreateUser struct {
//		OrgID  string `path:"org"`
//		DryRun bool   `query:"dry_run"`
//		Email  string `json:"email" validate:"required,email"`
//	}
//
//	api.Post("/orgs/{org}/users", zinc.Typed(func(c *zinc.Context, in CreateUser) (User, error) {
//		return users.Create(c.Context(), in)
//	})).Status(http.StatusCreated)
//
// In must be a struct; use struct{} for a handler without input. Its fields
// bind like Bind().All, from the body and then tagged header, query, and path
// fields, so a value from the URL or a header is never replaced by a body key.
// A failure is a *BindError (400). The configured Validator then
// runs, and a failure is a *ValidationError (422). An error returned by fn
// goes to the error handler like any other.
//
// The response status is the one fn or middleware sets with c.Status, or
// else the one declared with Route.Status, or else 200, 204 for NoContent
// and 302 for Redirect. The spec documents the same status. A status of 200
// set with c.Status counts as unset on a route without a declared status.
// Out is written as JSON, except for Zinc's output types: NoContent, Text,
// HTML, Bytes, File, Stream and Redirect, each written and documented as
// what it is. Use NoContent as Out for a response without a body. If fn
// writes the response itself, its output is ignored.
//
// A field of Out tagged header is sent as that response header, and needs
// json:"-" so it isn't also in the body:
//
//	type Created struct {
//		Location string `header:"Location" json:"-"`
//		ID       string `json:"id"`
//	}
//
// An empty string, nil pointer or slice, or zero time.Time sends nothing; a
// slice sends a value per element. A *http.Cookie or []*http.Cookie field
// tagged header:"Set-Cookie" sets cookies.
func Typed[In, Out any](fn func(*Context, In) (Out, error)) HandlerFunc {
	if fn == nil {
		panic("zinc: Typed handler is nil")
	}
	inType := reflect.TypeFor[In]()
	if inType.Kind() != reflect.Struct {
		panic(fmt.Sprintf("zinc: Typed input must be a struct, not %s; use struct{} for no input", inType))
	}
	// Compile the binding plan now, so the first request doesn't pay for it,
	// and a field binding can't fill fails here rather than per request.
	if err := bindingPlanFor(inType).err; err != nil {
		panic(err.Error())
	}
	types := handlerTypes{in: inType, out: reflect.TypeFor[Out]()}
	bindInput := inType.NumField() > 0
	kind := kindOf(types.out)
	noContent := kind == outputNoContent
	// Only NoContent and Redirect answer other than 200 by default.
	defaulted := noContent || kind == outputRedirect
	// write is nil for JSON, the common case, so it costs nothing there.
	write := outputWriter[Out]()
	// Fields tagged header are sent as response headers; nil when Out has
	// none, which is the common case.
	var headers []outputHeader
	if write == nil && !noContent {
		headers = outputHeadersFor(types.out)
	}

	h := func(c *Context) error {
		if c.index == describeIndex {
			c.store[describeKey{}] = types
			return nil
		}
		var in In
		if bindInput {
			if err := c.bindTyped(&in); err != nil {
				return err
			}
		}
		out, err := fn(c, in)
		if err != nil || c.written {
			return err
		}
		if defaulted {
			c.applySuccessStatus(kind)
		}
		if noContent {
			return c.writeNoContent()
		}
		if c.app != nil && c.app.config.ValidateResponses {
			if err := c.validateOutput(out); err != nil {
				return err
			}
		}
		if write != nil {
			return write(c, out)
		}
		if headers != nil {
			return c.jsonWithHeaders(out, headers)
		}
		return c.JSON(out)
	}
	typedPCs.Store(handlerPC(h), struct{}{})
	return h
}

// handlerTypes is what a Typed handler declares: its input and output types.
type handlerTypes struct {
	in, out reflect.Type
}

// typedPCs holds the code pointer of every closure Typed returns, recorded as
// each is made. Closures from instantiations with the same GC shape share
// one, so a code pointer can't identify the types, but it does identify a
// Typed closure: no other code has it. Where the compiler inlines Typed, a
// call site gets its own copy of the code and so its own pointer; recording
// each closure as it's made covers that. The set grows with shapes and
// inlined call sites, not with calls.
var typedPCs sync.Map

// describeIndex marks the Context describeHandler passes. A request's index
// is never below -1, so no real request can look like it.
const describeIndex = -2

type describeKey struct{}

// describeHandler asks a Typed handler for its types without running it. A
// handler that isn't a Typed closure is never called.
func describeHandler(h HandlerFunc) (handlerTypes, bool) {
	if h == nil {
		return handlerTypes{}, false
	}
	if _, ok := typedPCs.Load(handlerPC(h)); !ok {
		return handlerTypes{}, false
	}
	return describe(h)
}

// describe runs h in describe mode. A handler that panics there isn't a
// Typed closure, so it's reported as untyped.
func describe(h HandlerFunc) (types handlerTypes, ok bool) {
	defer func() {
		if recover() != nil {
			types, ok = handlerTypes{}, false
		}
	}()
	c := &Context{index: describeIndex, store: map[any]any{}}
	_ = h(c)
	types, ok = c.store[describeKey{}].(handlerTypes)
	return types, ok
}

// bindTyped binds exactly as Bind().All does.
func (c *Context) bindTyped(v any) error {
	return bindRequest(c, v)
}

// declaredStatus returns the success status set with Route.Status for the
// matched route, or 0.
func (c *Context) declaredStatus() int {
	if c.routeIndexed && c.app != nil && c.app.router != nil {
		return int(c.app.router.routeMetaAt(uint32(c.routeIndex)).status)
	}
	return int(c.routeInfo.status)
}

// successStatus is the status a route answers with when neither a handler
// nor middleware chose one: declared, from Route.Status, or else 204 for
// NoContent, 302 for Redirect and 200 for anything else. The response and
// the spec both use it.
func successStatus(kind outputKind, declared int) int {
	switch {
	case declared != 0:
		return declared
	case kind == outputNoContent:
		return StatusNoContent
	case kind == outputRedirect:
		return StatusFound
	}
	return StatusOK
}

// applySuccessStatus sets successStatus unless a handler or middleware
// chose another status. The route sets its declared status, or 200, before
// anything runs, so a status still at that value wasn't chosen.
func (c *Context) applySuccessStatus(kind outputKind) {
	declared := c.declaredStatus()
	unset := declared
	if unset == 0 {
		unset = StatusOK
	}
	if c.status == unset {
		c.status = successStatus(kind, declared)
	}
}

// checkRedirectStatus panics when out is Redirect and status isn't a
// redirect, a status the route could never send.
func (r *routeTable) checkRedirectStatus(index uint32, out reflect.Type, status int) {
	if status == 0 || kindOf(out) != outputRedirect || (status >= 300 && status <= 399) {
		return
	}
	info := r.routeInfos[index]
	panic(fmt.Sprintf("zinc: route %s %s has a Redirect output, so its status must be 301, 302, 303, 307 or 308, not %d", info.method, info.path, status))
}

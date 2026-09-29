// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"reflect"
	"sync"
)

// NoContent is the output type of a typed handler that sends no body. The
// route answers 204 No Content, or the status declared with Route.Status.
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
// The response status is 200, or the status declared with Route.Status, or
// the one fn sets with c.Status. Use NoContent as Out for a response without
// a body; it answers 204 unless another status is declared. If fn writes the
// response itself, its output is ignored.
func Typed[In, Out any](fn func(*Context, In) (Out, error)) HandlerFunc {
	if fn == nil {
		panic("zinc: Typed handler is nil")
	}
	inType := reflect.TypeFor[In]()
	if inType.Kind() != reflect.Struct {
		panic(fmt.Sprintf("zinc: Typed input must be a struct, not %s; use struct{} for no input", inType))
	}
	// Compile the binding plan now, so the first request doesn't pay for it.
	bindingPlanFor(inType)
	types := handlerTypes{in: inType, out: reflect.TypeFor[Out]()}
	bindInput := inType.NumField() > 0
	_, noContent := any(*new(Out)).(NoContent)

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
		if c.status == StatusOK {
			if declared := c.declaredStatus(); declared != 0 {
				c.status = declared
			} else if noContent {
				c.status = StatusNoContent
			}
		}
		if noContent {
			return c.NoContent()
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
	c := &Context{index: describeIndex, store: map[any]any{}}
	_ = h(c)
	types, ok := c.store[describeKey{}].(handlerTypes)
	return types, ok
}

// bindTyped binds like Bind().All and also binds header fields, after the
// body and before query and path values.
func (c *Context) bindTyped(v any) error {
	return bindRequest(c, v, true)
}

// declaredStatus returns the success status set with Route.Status for the
// matched route, or 0.
func (c *Context) declaredStatus() int {
	if c.routeIndexed && c.app != nil && c.app.router != nil {
		return int(c.app.router.routeMetaAt(uint32(c.routeIndex)).status)
	}
	return int(c.routeInfo.status)
}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"fmt"
	"reflect"
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
// bind like Bind().All, from tagged path, query, and header fields and the
// body, and a failure is a *BindError (400). The configured Validator then
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
	plan := bindingPlanFor(inType)
	bindInput := inType.NumField() > 0
	_, noContent := any(*new(Out)).(NoContent)

	return func(c *Context) error {
		var in In
		if bindInput {
			if err := c.bindTyped(&in, plan); err != nil {
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
}

// bindTyped binds header fields, which Bind().All leaves out, and then
// everything else as Bind().All does, which validates last.
func (c *Context) bindTyped(v any, plan *bindingPlan) error {
	if len(plan.headerFields) > 0 && c.request != nil {
		if err := bindFieldsFromHeader(reflect.ValueOf(v).Elem(), plan.headerFields, c.request.Header); err != nil {
			return wrapBindError("header", err)
		}
	}
	return bindAll(c, v)
}

// declaredStatus returns the success status set with Route.Status for the
// matched route, or 0.
func (c *Context) declaredStatus() int {
	if c.routeIndexed && c.app != nil && c.app.router != nil {
		return int(c.app.router.routeMetaAt(uint32(c.routeIndex)).status)
	}
	return int(c.routeInfo.status)
}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package marks lets Zinc's own middleware tell Zinc what it needs at
// registration: to run before routing, such as rewrite, or to answer CORS
// preflight requests, such as cors. It's internal: nothing else can mark
// middleware.
//
// Zinc asks a middleware by calling it with a probe, a Context whose request
// is Probe, and the middleware answers by returning its Marks as the error.
// Each instance answers for itself. Marks kept by code address would be
// shared by every closure a function makes, so marking one Skip wrapper
// would mark them all. Zinc probes only closures whose code is known to
// answer (see Answers), so no other middleware runs outside a request, and
// probes happen at registration, never per request.
package marks

import (
	"net/http"
	"reflect"
	"sync"
)

// Probe is the request of the Context Zinc probes middleware with. No real
// request is ever this one.
var Probe = &http.Request{}

// Marks is a middleware's answer to a probe.
type Marks struct {
	// Prerouting names middleware that only works before routing, so it
	// can't be registered on a group.
	Prerouting string
	// Preflight is set on middleware that also answers CORS preflight
	// requests, so Zinc runs it on the automatic OPTIONS response.
	Preflight bool
}

func (*Marks) Error() string { return "zinc: middleware marks" }

// StaticDirectory reports whether a static mount of the app serving c, a
// *zinc.Context, serves path as a directory. Zinc sets it. A directory's URL
// ends with a slash, so trailingslash leaves such a path alone rather than
// loop with the directory's redirect.
var StaticDirectory func(c any, path string) bool

var answering sync.Map // code pointer → struct{}

// Answers records that closures with mw's code answer a probe. It's a fact
// about the code, not the instance: the instance decides what it answers.
// Call it on every instance a constructor returns; where the compiler
// inlines the constructor, a call site gets its own copy of the code.
func Answers(mw any) {
	if pc := codePointer(mw); pc != 0 {
		answering.Store(pc, struct{}{})
	}
}

// CanAnswer reports whether mw's code answers a probe, so it's safe to
// probe.
func CanAnswer(mw any) bool {
	pc := codePointer(mw)
	if pc == 0 {
		return false
	}
	_, ok := answering.Load(pc)
	return ok
}

func codePointer(fn any) uintptr {
	v := reflect.ValueOf(fn)
	if !v.IsValid() || v.Kind() != reflect.Func || v.IsNil() {
		return 0
	}
	return v.Pointer()
}

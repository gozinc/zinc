// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package prerouting records middleware that only works before routing, such
// as redirect and rewrite, so Zinc can refuse to register it on a group. It's
// internal: Zinc's own middleware marks itself, and nothing else can.
package prerouting

import (
	"reflect"
	"sync"
)

var marked sync.Map // code pointer → middleware name

// Mark records that mw must run before routing. Call it on every instance
// a constructor returns: when the constructor is inlined, the compiler can
// give each call site its own copy of the closure's code, and so its own
// code pointer. Marking the instance itself is always exact.
func Mark(mw any, name string) {
	if pc := codePointer(mw); pc != 0 {
		marked.Store(pc, name)
	}
}

// Name returns the middleware's name when mw was marked.
func Name(mw any) (string, bool) {
	name, ok := marked.Load(codePointer(mw))
	if !ok {
		return "", false
	}
	return name.(string), true
}

func codePointer(fn any) uintptr {
	v := reflect.ValueOf(fn)
	if !v.IsValid() || v.Kind() != reflect.Func || v.IsNil() {
		return 0
	}
	return v.Pointer()
}

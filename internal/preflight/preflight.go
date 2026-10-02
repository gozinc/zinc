// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

// Package preflight records middleware that should also answer CORS
// preflight requests for the routes it guards, such as cors, so Zinc can run
// it on the automatic OPTIONS response. It's separate from prerouting: a mark
// here never makes middleware run before routing. It's internal: Zinc's own
// middleware marks itself, and nothing else can.
package preflight

import (
	"reflect"
	"sync"
)

var marked sync.Map // code pointer → struct{}

// Mark records that mw handles preflight requests. Call it on every instance
// a constructor returns; see prerouting.Mark for why.
func Mark(mw any) {
	if pc := codePointer(mw); pc != 0 {
		marked.Store(pc, struct{}{})
	}
}

// Is reports whether mw was marked.
func Is(mw any) bool {
	pc := codePointer(mw)
	if pc == 0 {
		return false
	}
	_, ok := marked.Load(pc)
	return ok
}

func codePointer(fn any) uintptr {
	v := reflect.ValueOf(fn)
	if !v.IsValid() || v.Kind() != reflect.Func || v.IsNil() {
		return 0
	}
	return v.Pointer()
}

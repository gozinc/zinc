// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"math/bits"
	"sort"
	"strings"
)

// Methods: standard methods are bits in a methodMask, so a terminal's method
// table and a 405's Allow header need no allocation. Extension methods, such
// as PURGE, sit beside them in a short sorted list.

type allowedMethodSet struct {
	// Standard methods use a mask; extension methods allocate only when present.
	mask  methodMask
	extra []string
}

var routeMethods = []string{
	MethodGet,
	MethodHead,
	MethodPost,
	MethodPut,
	MethodPatch,
	MethodDelete,
	MethodOptions,
	MethodConnect,
	MethodTrace,
}

const routeMethodCount = 9

type methodMask uint16

const (
	methodMaskGet methodMask = 1 << iota
	methodMaskHead
	methodMaskPost
	methodMaskPut
	methodMaskPatch
	methodMaskDelete
	methodMaskOptions
	methodMaskConnect
	methodMaskTrace
)

const allowHeaderTableSize = 1 << 9

var allowHeaderByMask [allowHeaderTableSize]string

func init() {
	// Every standard-method combination has a canonical, allocation-free Allow value.
	for raw := 0; raw < len(allowHeaderByMask); raw++ {
		allowHeaderByMask[raw] = buildAllowHeader(methodMask(raw))
	}
}

func singleBitIndex(mask methodMask) int {
	raw := uint16(mask)
	if raw == 0 || raw&(raw-1) != 0 {
		return -1
	}
	index := bits.TrailingZeros16(raw)
	if index >= routeMethodCount {
		return -1
	}
	return index
}

func (s *allowedMethodSet) addMethod(method string) {
	if method == "" {
		return
	}
	if mask := methodMaskFor(method); mask != 0 {
		s.mask |= mask
		return
	}
	for _, existing := range s.extra {
		if existing == method {
			return
		}
	}
	s.extra = append(s.extra, method)
}

func (s allowedMethodSet) empty() bool {
	return s.mask == 0 && len(s.extra) == 0
}

func (s allowedMethodSet) withAutomatic(autoHead, autoOptions bool) allowedMethodSet {
	if s.empty() {
		return allowedMethodSet{}
	}
	if autoHead && s.mask&methodMaskGet != 0 {
		s.mask |= methodMaskHead
	}
	if autoOptions {
		s.mask |= methodMaskOptions
	}
	return s
}

func (s allowedMethodSet) header(autoHead, autoOptions bool) string {
	s = s.withAutomatic(autoHead, autoOptions)
	if s.empty() {
		return ""
	}
	if len(s.extra) == 0 {
		return allowHeader(s.mask)
	}
	return buildAllowHeaderWithExtra(s.mask, sortedExtra(s.extra))
}

func methodMaskFor(method string) methodMask {
	switch method {
	case MethodGet:
		return methodMaskGet
	case MethodHead:
		return methodMaskHead
	case MethodPost:
		return methodMaskPost
	case MethodPut:
		return methodMaskPut
	case MethodPatch:
		return methodMaskPatch
	case MethodDelete:
		return methodMaskDelete
	case MethodOptions:
		return methodMaskOptions
	case MethodConnect:
		return methodMaskConnect
	case MethodTrace:
		return methodMaskTrace
	default:
		return 0
	}
}

func allowHeader(mask methodMask) string {
	if mask == 0 {
		return ""
	}
	if int(mask) < len(allowHeaderByMask) {
		return allowHeaderByMask[int(mask)]
	}
	return buildAllowHeader(mask)
}

func buildAllowHeader(mask methodMask) string {
	if mask == 0 {
		return ""
	}
	return buildAllowHeaderWithExtra(mask, nil)
}

func buildAllowHeaderWithExtra(mask methodMask, extra []string) string {
	if mask == 0 && len(extra) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, method := range routeMethods {
		if mask&methodMaskFor(method) == 0 {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(method)
	}
	for _, method := range extra {
		if builder.Len() > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(method)
	}
	return builder.String()
}

// nodeMethods is a terminal node's routes, by method.
type nodeMethods struct {
	std    [routeMethodCount]*radixRoute
	custom []customRoute
}

type customRoute struct {
	method string
	route  *radixRoute
}

func (m *nodeMethods) get(slot int, method string) *radixRoute {
	if slot >= 0 {
		return m.std[slot]
	}
	for i := range m.custom {
		if m.custom[i].method == method {
			return m.custom[i].route
		}
	}
	return nil
}

// set records route for the method, reporting false if it already has one.
func (m *nodeMethods) set(slot int, method string, route *radixRoute) bool {
	if m.get(slot, method) != nil {
		return false
	}
	if slot >= 0 {
		m.std[slot] = route
		return true
	}
	m.custom = append(m.custom, customRoute{method: method, route: route})
	return true
}

// addAllowed adds the methods whose route here takes captured parameters.
func (m *nodeMethods) addAllowed(into *allowedMethodSet, captured int) {
	for slot, route := range m.std {
		if route != nil && int(route.paramCount) == captured {
			into.mask |= methodMaskFor(routeMethods[slot])
		}
	}
	for _, c := range m.custom {
		if int(c.route.paramCount) == captured {
			into.addMethod(c.method)
		}
	}
}

// sortedExtra returns custom methods in sorted order for Allow.
func sortedExtra(extra []string) []string {
	if len(extra) < 2 || sort.StringsAreSorted(extra) {
		return extra
	}
	sorted := append([]string(nil), extra...)
	sort.Strings(sorted)
	return sorted
}

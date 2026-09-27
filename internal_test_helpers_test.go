// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import "sync/atomic"

// These small adapters exist only to arrange test data and invoke production
// primitives. Routing and method-negotiation algorithms remain in production.
func (c *Context) setParam(key, value string) {
	c.ensurePathParamCapacity(c.paramCount + 1)
	c.pathParams[c.paramCount] = param{key: key, value: value, start: directParamStart}
	c.paramCount++
	c.paramRoute = nil
}
func (rc *routeCache) get(key routeCacheKey) (routeCacheEntry, bool) {
	return rc.getWithMask(key, methodMaskFor(key.method))
}
func (rc *routeCache) set(key routeCacheKey, entry routeCacheEntry) {
	rc.setWithMask(key, methodMaskFor(key.method), entry)
}
func (rc *routeCache) setMiss(key routeCacheKey, entry routeCacheEntry) {
	rc.setMissWithMask(key, methodMaskFor(key.method), entry)
}

// routeCacheMinRoutes sizes the route-cache tests' working sets.
const routeCacheMinRoutes = 64

func (rc *routeCache) getHot(key routeCacheKey) (routeCacheEntry, bool) {
	if rc == nil || atomic.LoadUint32(&rc.dirty) != 0 {
		return routeCacheEntry{}, false
	}
	if hot := rc.hot.Load(); hot != nil && hot.key == key {
		return hot.entry, true
	}
	return routeCacheEntry{}, false
}

// methodRoute is this node's route for the method, if it takes captured
// parameters.
func (n *radixNode) methodRoute(slot int, method string, captured int) *radixRoute {
	if n.methods == nil {
		return nil
	}
	route := n.methods.get(slot, method)
	if route == nil || int(route.paramCount) != captured {
		return nil
	}
	return route
}

func bindData(ptr any, data map[string][]string, tag string) error {
	val, plan, err := bindTargetPlan(ptr)
	if err != nil {
		return err
	}
	switch tag {
	case "path":
		return bindFieldsFromValues(val, plan.pathFields, data)
	case "query":
		return bindFieldsFromValues(val, plan.queryFields, data)
	case "form":
		return bindFieldsFromValues(val, plan.formFields, data)
	case "header":
		return bindFieldsFromValues(val, plan.headerFields, data)
	default:
		return nil
	}
}

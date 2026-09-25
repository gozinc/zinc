// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

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

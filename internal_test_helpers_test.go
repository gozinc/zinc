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

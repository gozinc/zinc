// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Validate checks the app's routes and spec without serving a request, so
// a mistake fails a test or a deploy rather than the first request that
// reaches it. Call it after registering routes:
//
//	if err := app.Validate(); err != nil {
//		log.Fatal(err)
//	}
//
// It builds every spec the app serves, which fails on routes OpenAPI can't
// tell apart, unknown security schemes and examples that don't match their
// types, and it reports what registration can't panic over:
//
//   - a header-tagged output field that's still in the JSON body, which
//     sends no header, unless the type is also the route's input;
//   - an openapi tag word other than readonly, writeonly and deprecated;
//   - a validate rule the validator doesn't enforce, on a type given to
//     Route.Input or Route.Output.
//
// The error lists every problem, one per line; nil means none.
func (a *App) Validate() error {
	var problems []error
	specs := a.specs
	if a.spec != nil {
		specs = append([]*servedSpec{a.spec}, specs...)
	}
	if len(specs) == 0 {
		specs = []*servedSpec{{app: a, cfg: a.config.OpenAPI}}
	}
	for _, spec := range specs {
		if _, err := a.OpenAPISpec(spec.cfg); err != nil {
			problems = append(problems, err)
		}
	}
	table := a.router
	for i, meta := range table.routeInfos {
		rd := table.routeDocs[uint32(i)]
		if rd == nil || meta.mounted {
			continue
		}
		route := meta.method + " " + meta.path
		if !rd.typed {
			if err := ruleSupportError(handlerTypes{in: rd.in, out: rd.out}, &a.config); err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", route, err))
			}
		}
		for _, t := range []reflect.Type{rd.in, rd.out} {
			for _, st := range ruleTypes(t) {
				for _, f := range reflect.VisibleFields(st) {
					if !f.IsExported() {
						continue
					}
					for _, word := range strings.Split(f.Tag.Get("openapi"), ",") {
						switch strings.TrimSpace(word) {
						case "", "readonly", "writeonly", "deprecated":
						default:
							problems = append(problems, fmt.Errorf("%s: %s.%s: unknown openapi tag word %q; want readonly, writeonly or deprecated", route, st, f.Name, strings.TrimSpace(word)))
						}
					}
				}
			}
		}
		// A type that's both the input and the output is echoed back, and
		// its header fields are input fields, so they belong in the body.
		if out := nestedStruct(rd.out); out != nil && kindOf(rd.out) == outputJSON && (rd.in == nil || base(rd.in) != out) {
			for _, f := range reflect.VisibleFields(out) {
				if name, ok := f.Tag.Lookup("header"); ok && name != "-" && f.IsExported() && f.Tag.Get("json") != "-" {
					problems = append(problems, fmt.Errorf(`%s: %s.%s has header:%q but is sent in the JSON body, so no header is sent; add json:"-" to send it as a header`, route, out, f.Name, name))
				}
			}
		}
	}
	return errors.Join(problems...)
}

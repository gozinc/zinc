// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// operationHook is a Route.Operation hook with the operation it edits.
type operationHook struct {
	path, method string
	fn           func(op map[string]any)
}

// applySpecHooks runs Extensions, the Route.Operation hooks and Mutate on
// the spec as decoded JSON, checks the result, and returns it with the
// original member order kept and new members after, sorted.
func applySpecHooks(doc *oaDocument, cfg OpenAPIConfig) (any, error) {
	body, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	ordered, err := decodeOrdered(dec)
	if err != nil {
		return nil, err
	}
	spec := plainJSON(ordered).(map[string]any)
	for key, value := range cfg.Extensions {
		spec[key] = value
	}
	paths, _ := spec["paths"].(map[string]any)
	for _, hook := range doc.hooks {
		item, _ := paths[hook.path].(map[string]any)
		if op, ok := item[hook.method].(map[string]any); ok {
			hook.fn(op)
		}
	}
	if cfg.Mutate != nil {
		if err := cfg.Mutate(spec); err != nil {
			return nil, fmt.Errorf("zinc: OpenAPIConfig.Mutate: %w", err)
		}
	}
	// A hook may add Go values, such as a struct; encode them as the spec
	// will be, so the check sees what clients get.
	body, err = json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("zinc: the spec after its hooks isn't JSON: %w", err)
	}
	var checked map[string]any
	if err := json.Unmarshal(body, &checked); err != nil {
		return nil, err
	}
	if err := checkSpecShape(checked); err != nil {
		return nil, fmt.Errorf("zinc: the spec after its hooks: %w", err)
	}
	dec = json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	edited, err := decodeOrdered(dec)
	if err != nil {
		return nil, err
	}
	return keepOrder(ordered, edited), nil
}

// plainJSON turns decodeOrdered's objects into maps, for hooks.
func plainJSON(v any) any {
	switch v := v.(type) {
	case orderedMap[any]:
		m := make(map[string]any, len(v))
		for _, e := range v {
			m[e.key] = plainJSON(e.value)
		}
		return m
	case []any:
		for i := range v {
			v[i] = plainJSON(v[i])
		}
	}
	return v
}

// keepOrder orders edited's object members as original had them, with new
// members after, sorted by name.
func keepOrder(original, edited any) any {
	switch e := edited.(type) {
	case orderedMap[any]:
		o, _ := original.(orderedMap[any])
		index := make(map[string]int, len(e))
		for i, entry := range e {
			index[entry.key] = i
		}
		out := make(orderedMap[any], 0, len(e))
		seen := map[string]bool{}
		for _, entry := range o {
			if i, ok := index[entry.key]; ok {
				out = append(out, orderedEntry[any]{entry.key, keepOrder(entry.value, e[i].value)})
				seen[entry.key] = true
			}
		}
		var added []orderedEntry[any]
		for _, entry := range e {
			if !seen[entry.key] {
				added = append(added, orderedEntry[any]{entry.key, keepOrder(nil, entry.value)})
			}
		}
		sort.Slice(added, func(i, j int) bool { return added[i].key < added[j].key })
		return append(out, added...)
	case []any:
		o, _ := original.([]any)
		for i := range e {
			var prev any
			if i < len(o) {
				prev = o[i]
			}
			e[i] = keepOrder(prev, e[i])
		}
	}
	return edited
}

// checkSpecShape checks what hooks could break: the version, the info
// object, and that every local $ref resolves.
func checkSpecShape(spec map[string]any) error {
	if spec["openapi"] != "3.1.0" {
		return fmt.Errorf(`"openapi" is %v, want "3.1.0"`, spec["openapi"])
	}
	info, _ := spec["info"].(map[string]any)
	if title, _ := info["title"].(string); title == "" {
		return errors.New("info.title is missing")
	}
	if version, _ := info["version"].(string); version == "" {
		return errors.New("info.version is missing")
	}
	if paths, ok := spec["paths"]; ok {
		if _, ok := paths.(map[string]any); !ok {
			return errors.New(`"paths" isn't an object`)
		}
	}
	var problems []string
	var walk func(v any, at string)
	walk = func(v any, at string) {
		switch v := v.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok && strings.HasPrefix(ref, "#/") && !resolves(spec, ref) {
				problems = append(problems, fmt.Sprintf("%s: $ref %s doesn't resolve", at, ref))
			}
			for key, member := range v {
				walk(member, at+"/"+strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1"))
			}
		case []any:
			for i, element := range v {
				walk(element, fmt.Sprintf("%s/%d", at, i))
			}
		}
	}
	walk(spec, "#")
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// resolves reports whether a local JSON pointer finds a value in spec.
func resolves(spec map[string]any, ref string) bool {
	var v any = spec
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		if v, ok = m[part]; !ok {
			return false
		}
	}
	return true
}

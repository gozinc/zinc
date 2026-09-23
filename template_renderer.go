// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"reflect"
	"strings"
	texttemplate "text/template"
)

// Template renderer configuration and lookup errors.
var (
	ErrTemplateEngineNotConfigured = errors.New("template engine is not configured")
	ErrTemplateNameRequired        = errors.New("template name is required")
	ErrTemplateNotFound            = errors.New("template not found")
)

// TemplateExecutor is implemented by html/template and text/template sets.
type TemplateExecutor interface {
	ExecuteTemplate(w io.Writer, name string, data any) error
}

// TemplateRenderer adapts parsed templates to Zinc's Renderer contract.
type TemplateRenderer struct {
	Engine       TemplateExecutor
	nameSuffixes []string
}

// TemplateRendererOption configures template name resolution.
type TemplateRendererOption func(*TemplateRenderer)

// NewTemplateRenderer wraps a parsed template set.
func NewTemplateRenderer(engine TemplateExecutor, opts ...TemplateRendererOption) *TemplateRenderer {
	renderer := &TemplateRenderer{
		Engine: engine,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(renderer)
		}
	}
	return renderer
}

// NewHTMLTemplateRenderer wraps an html/template set.
func NewHTMLTemplateRenderer(engine *htmltemplate.Template, opts ...TemplateRendererOption) *TemplateRenderer {
	return NewTemplateRenderer(engine, opts...)
}

// NewTextTemplateRenderer wraps a text/template set.
func NewTextTemplateRenderer(engine *texttemplate.Template, opts ...TemplateRendererOption) *TemplateRenderer {
	return NewTemplateRenderer(engine, opts...)
}

// WithTemplateSuffixes configures fallback suffixes tried after an exact name.
func WithTemplateSuffixes(suffixes ...string) TemplateRendererOption {
	cleaned := cleanTemplateSuffixes(suffixes)
	return func(renderer *TemplateRenderer) {
		renderer.nameSuffixes = append(renderer.nameSuffixes[:0], cleaned...)
	}
}

// Render executes the first matching exact or suffixed template name.
func (r *TemplateRenderer) Render(w io.Writer, name string, data any, _ *Context) error {
	if r == nil || r.Engine == nil {
		return ErrTemplateEngineNotConfigured
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return ErrTemplateNameRequired
	}

	candidates := templateCandidates(name, r.nameSuffixes)
	if resolved, found, ok := lookupTemplateName(r.Engine, candidates); ok {
		if !found {
			return fmt.Errorf("%w: %s", ErrTemplateNotFound, name)
		}
		return r.Engine.ExecuteTemplate(w, resolved, data)
	}

	var firstErr error
	for _, candidate := range candidates {
		err := r.Engine.ExecuteTemplate(w, candidate, data)
		if err == nil {
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func cleanTemplateSuffixes(suffixes []string) []string {
	out := make([]string, 0, len(suffixes))
	seen := make(map[string]struct{}, len(suffixes))

	for _, suffix := range suffixes {
		suffix = strings.TrimSpace(suffix)
		if suffix == "" {
			continue
		}
		if !strings.HasPrefix(suffix, ".") {
			suffix = "." + suffix
		}
		if _, exists := seen[suffix]; exists {
			continue
		}
		seen[suffix] = struct{}{}
		out = append(out, suffix)
	}
	return out
}

func templateCandidates(name string, suffixes []string) []string {
	candidates := []string{name}
	seen := map[string]struct{}{name: {}}

	for _, suffix := range suffixes {
		candidate := name
		if !strings.HasSuffix(candidate, suffix) {
			candidate = name + suffix
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}
	return candidates
}

// lookupTemplateName uses an optional Lookup method when available. Engines
// exposing only ExecuteTemplate retain compatibility through execution fallback.
func lookupTemplateName(engine TemplateExecutor, candidates []string) (string, bool, bool) {
	method := reflect.ValueOf(engine).MethodByName("Lookup")
	if !method.IsValid() {
		return "", false, false
	}
	methodType := method.Type()
	if methodType.NumIn() != 1 || methodType.In(0).Kind() != reflect.String || methodType.NumOut() != 1 {
		return "", false, false
	}

	for _, candidate := range candidates {
		out := method.Call([]reflect.Value{reflect.ValueOf(candidate)})[0]
		if !isNilValue(out) {
			return candidate, true, true
		}
	}
	return "", false, true
}

func isNilValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

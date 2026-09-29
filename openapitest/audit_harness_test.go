// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package openapitest

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/0mjs/zinc"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The audit checks Zinc's OpenAPI generator scenario by scenario. Each
// scenario is a small app plus what a correct spec says about it, taken from
// OpenAPI and HTTP, not from what Zinc does. Every scenario runs to the end,
// collecting its findings; then testdata/audit-baseline.json decides which
// are accepted (known gaps with the phase that fixes them, and intended
// behavior). Anything else fails the test.
//
// Run: go test -run TestAudit ./...
// With AUDIT_OUT=/tmp/audit it also writes results.json and one spec per
// scenario there. After a fix, -update-audit-baseline rewrites the findings.

type scenario struct {
	id, area, title string
	// build returns the app, and the config to build its spec with.
	build func() (*zinc.App, zinc.OpenAPIConfig)
	// expect checks what the spec says. It reports findings through f.
	expect func(f *findings, s spec)
	// probes are real requests; see probe.
	probes []probe
	// note records context for the report.
	note string
}

// probe sends a request. The response body must validate against the schema
// the spec documents for its status; a request body the server accepts (a
// 2xx) must validate against the documented request schema.
type probe struct {
	method, target, body, contentType string
	header                            http.Header
	status                            int
}

type findings struct {
	list []string
}

func (f *findings) add(format string, args ...any) {
	f.list = append(f.list, fmt.Sprintf(format, args...))
}

// spec is a parsed spec with small helpers for expectations.
type spec struct {
	raw map[string]any
}

func (s spec) op(method, path string) map[string]any {
	paths, _ := s.raw["paths"].(map[string]any)
	item, _ := paths[path].(map[string]any)
	op, _ := item[strings.ToLower(method)].(map[string]any)
	return op
}

func (s spec) hasPath(path string) bool {
	paths, _ := s.raw["paths"].(map[string]any)
	_, ok := paths[path]
	return ok
}

func (s spec) param(method, path, in, name string) map[string]any {
	for _, p := range asList(s.op(method, path)["parameters"]) {
		pm, _ := p.(map[string]any)
		if pm["in"] == in && pm["name"] == name {
			return pm
		}
	}
	return nil
}

func (s spec) component(name string) map[string]any {
	c, _ := s.raw["components"].(map[string]any)
	sch, _ := c["schemas"].(map[string]any)
	m, _ := sch[name].(map[string]any)
	return m
}

func (s spec) components() []string {
	c, _ := s.raw["components"].(map[string]any)
	sch, _ := c["schemas"].(map[string]any)
	var out []string
	for k := range sch {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// resolve follows a $ref to its component.
func (s spec) resolve(sch map[string]any) map[string]any {
	if ref, ok := sch["$ref"].(string); ok {
		return s.component(strings.TrimPrefix(ref, "#/components/schemas/"))
	}
	return sch
}

func (s spec) response(method, path string, status int) map[string]any {
	r, _ := s.op(method, path)["responses"].(map[string]any)
	m, _ := r[strconv.Itoa(status)].(map[string]any)
	return m
}

func (s spec) responseSchema(method, path string, status int) map[string]any {
	return s.resolve(mediaSchema(s.response(method, path, status), "application/json"))
}

func (s spec) requestSchema(method, path, mediaType string) map[string]any {
	rb, _ := s.op(method, path)["requestBody"].(map[string]any)
	return s.resolve(mediaSchema(rb, mediaType))
}

func mediaSchema(holder map[string]any, mediaType string) map[string]any {
	content, _ := holder["content"].(map[string]any)
	mt, _ := content[mediaType].(map[string]any)
	sch, _ := mt["schema"].(map[string]any)
	return sch
}

func props(sch map[string]any) map[string]any { p, _ := sch["properties"].(map[string]any); return p }

func prop(sch map[string]any, name string) map[string]any {
	p, _ := props(sch)[name].(map[string]any)
	return p
}

func required(sch map[string]any) []string {
	var out []string
	for _, r := range asList(sch["required"]) {
		out = append(out, fmt.Sprint(r))
	}
	return out
}

func asList(v any) []any { l, _ := v.([]any); return l }

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// typeOf returns a schema's type list, following a nullable anyOf.
func typeOf(sch map[string]any) []string {
	switch t := sch["type"].(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, x := range t {
			out = append(out, fmt.Sprint(x))
		}
		return out
	}
	return nil
}

func typeIs(sch map[string]any, want string) bool { return has(typeOf(sch), want) }

// auditResult is one scenario's outcome, as written to results.json.
type auditResult struct {
	ID, Area, Title, Note string
	Valid                 []string // OpenAPI 3.1 / JSON Schema validity
	Expect                []string // what the spec should say
	Conformance           []string // real requests and responses
	Probes                int
}

var (
	auditMu      sync.Mutex
	auditResults []auditResult
)

const auditURL = "https://zinc.test/audit.json"

func runScenario(t *testing.T, sc scenario, out string) auditResult {
	res := auditResult{ID: sc.id, Area: sc.area, Title: sc.title, Note: sc.note, Probes: len(sc.probes)}
	var app *zinc.App
	var cfg zinc.OpenAPIConfig
	func() {
		defer func() {
			if r := recover(); r != nil {
				res.Valid = append(res.Valid, fmt.Sprintf("building the app panicked: %v", r))
			}
		}()
		app, cfg = sc.build()
	}()
	if app == nil {
		return res
	}
	if cfg.Title == "" {
		cfg.Title, cfg.Version = "Audit", "1"
	}
	raw, err := app.OpenAPISpec(cfg)
	if err != nil {
		res.Valid = append(res.Valid, "OpenAPISpec: "+err.Error())
		return res
	}
	if out != "" {
		_ = os.WriteFile(filepath.Join(out, "specs", sc.id+".json"), raw, 0o644)
	}

	// 1. Validity.
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		res.Valid = append(res.Valid, "not JSON: "+err.Error())
		return res
	}
	res.Valid = append(res.Valid, validate31(doc)...)
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	if err := c.AddResource(auditURL, doc); err != nil {
		res.Valid = append(res.Valid, "schema resource: "+err.Error())
		return res
	}
	for _, ptr := range schemaPointers(doc, "") {
		if _, err := c.Compile(auditURL + "#" + ptr); err != nil {
			res.Valid = append(res.Valid, fmt.Sprintf("schema at %s: %v", ptr, firstLine(err)))
		}
	}

	// 2. Expectations.
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	s := spec{raw: m}
	if sc.expect != nil {
		f := &findings{}
		sc.expect(f, s)
		res.Expect = f.list
	}

	// 3. Conformance.
	for _, p := range sc.probes {
		func() {
			defer func() {
				if r := recover(); r != nil {
					res.Conformance = append(res.Conformance, fmt.Sprintf("%s %s: the handler's panic reached net/http: %v", p.method, p.target, r))
				}
			}()
			res.Conformance = append(res.Conformance, runProbe(app, s, c, p)...)
		}()
	}
	return res
}

func runProbe(app *zinc.App, s spec, c *jsonschema.Compiler, p probe) []string {
	var out []string
	name := p.method + " " + p.target
	var body io.Reader
	if p.body != "" {
		body = strings.NewReader(p.body)
	}
	r := httptest.NewRequest(p.method, p.target, body)
	if p.contentType != "" {
		r.Header.Set("Content-Type", p.contentType)
	} else if p.body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range p.header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if p.status != 0 && w.Code != p.status {
		out = append(out, fmt.Sprintf("%s: status %d, want %d (%s)", name, w.Code, p.status, trim(w.Body.String())))
	}

	route := ""
	paths, _ := s.raw["paths"].(map[string]any)
	for tmpl := range paths {
		if matchesTemplate(tmpl, r.URL.Path) {
			if _, ok := paths[tmpl].(map[string]any)[strings.ToLower(p.method)]; ok {
				route = tmpl
			}
		}
	}
	if route == "" {
		return append(out, name+": no matching operation in the spec")
	}
	opPtr := "#/paths/" + escape(route) + "/" + strings.ToLower(p.method)

	// The request body, when the server accepted it.
	if p.body != "" && w.Code < 300 {
		mt := strings.Split(r.Header.Get("Content-Type"), ";")[0]
		if mt == "application/json" {
			if s.requestSchema(p.method, route, "application/json") == nil {
				out = append(out, name+": accepted a JSON body the spec doesn't document")
			} else if sch, err := c.Compile(auditURL + opPtr + "/requestBody/content/application~1json/schema"); err != nil {
				out = append(out, name+": request schema: "+firstLine(err))
			} else {
				var v any
				_ = json.Unmarshal([]byte(p.body), &v)
				if err := sch.Validate(v); err != nil {
					out = append(out, fmt.Sprintf("%s: accepted a body its request schema rejects: %s", name, firstLine(err)))
				}
			}
		}
	}

	// The response: its status, or the default response.
	resp := s.response(p.method, route, w.Code)
	if resp == nil {
		resp, _ = s.op(p.method, route)["responses"].(map[string]any)["default"].(map[string]any)
	}
	if resp == nil {
		return append(out, fmt.Sprintf("%s: status %d isn't documented", name, w.Code))
	}
	content, hasContent := resp["content"].(map[string]any)
	if _, anything := content["*/*"]; anything && len(content) == 1 {
		return out // any body, or none, of any media type
	}
	if w.Body.Len() == 0 || p.method == http.MethodHead {
		if hasContent && w.Code != http.StatusNoContent && p.method != http.MethodHead {
			out = append(out, fmt.Sprintf("%s: documented a body for %d, sent none", name, w.Code))
		}
		return out
	}
	mt := strings.Split(w.Header().Get("Content-Type"), ";")[0]
	if !hasContent {
		return append(out, fmt.Sprintf("%s: sent a %s body for %d, documented none", name, mt, w.Code))
	}
	if _, ok := content[mt]; !ok {
		var docd []string
		for k := range content {
			docd = append(docd, k)
		}
		return append(out, fmt.Sprintf("%s: sent %s, documented %v", name, mt, docd))
	}
	if mt != "application/json" {
		return out
	}
	sch, err := c.Compile(auditURL + opPtr + "/responses/" + strconv.Itoa(w.Code) + "/content/application~1json/schema")
	if err != nil {
		return append(out, name+": response schema: "+firstLine(err))
	}
	var v any
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		return append(out, name+": response isn't JSON")
	}
	if err := sch.Validate(v); err != nil {
		out = append(out, fmt.Sprintf("%s: response doesn't match its schema: %s — body %s", name, firstLine(err), trim(w.Body.String())))
	}
	return out
}

var oas31 struct {
	once sync.Once
	sch  *jsonschema.Schema
	err  error
}

func validate31(doc any) []string {
	oas31.once.Do(func() {
		f, err := os.Open("testdata/oas-3.1-schema.json")
		if err != nil {
			oas31.err = err
			return
		}
		defer f.Close()
		v, err := jsonschema.UnmarshalJSON(f)
		if err != nil {
			oas31.err = err
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(oasSchemaURL, v); err != nil {
			oas31.err = err
			return
		}
		oas31.sch, oas31.err = c.Compile(oasSchemaURL)
	})
	if oas31.err != nil {
		return []string{"loading the 3.1 schema: " + oas31.err.Error()}
	}
	if err := oas31.sch.Validate(doc); err != nil {
		return []string{"not valid OpenAPI 3.1: " + oneLine(err.Error())}
	}
	return nil
}

func matchesTemplate(tmpl, path string) bool {
	ts, ps := strings.Split(tmpl, "/"), strings.Split(path, "/")
	if len(ts) > 0 && strings.HasSuffix(tmpl, "}") && strings.Contains(tmpl, "{path}") && len(ps) >= len(ts) {
		ps = append(ps[:len(ts)-1], strings.Join(ps[len(ts)-1:], "/"))
	}
	if len(ts) != len(ps) {
		return false
	}
	for i := range ts {
		if !strings.HasPrefix(ts[i], "{") && !strings.EqualFold(ts[i], ps[i]) {
			return false
		}
	}
	return true
}

func firstLine(err error) string { return oneLine(err.Error()) }

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return trim(s)
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}

// The baseline gates the audit. It lists each scenario that is allowed
// findings: why, and exactly which findings. Every other scenario must have
// none. Findings starting "info:" are observations and never gate. A new
// finding, or one that disappears, fails the test: classify the new one, or
// remove the entry once its fix lands. Run with -update-audit-baseline to
// rewrite the findings; new entries get status "new", which also fails until
// someone classifies them.
var updateBaseline = flag.Bool("update-audit-baseline", false, "rewrite testdata/audit-baseline.json from this run")

const baselinePath = "testdata/audit-baseline.json"

// baselineEntry is one scenario's accepted findings.
type baselineEntry struct {
	// Status is "gap" (a known shortfall, with the phase that fixes it),
	// "design" (the behavior is intended) or "tooling".
	Status   string   `json:"status"`
	Phase    string   `json:"phase,omitempty"`
	Reason   string   `json:"reason"`
	Findings []string `json:"findings"`
}

func gatedFindings(res auditResult) []string {
	var out []string
	for _, list := range [][]string{res.Valid, res.Expect, res.Conformance} {
		for _, f := range list {
			if !strings.HasPrefix(f, "info:") {
				out = append(out, f)
			}
		}
	}
	return out
}

func loadBaseline(t *testing.T) map[string]baselineEntry {
	t.Helper()
	b, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("reading the audit baseline: %v", err)
	}
	var m map[string]baselineEntry
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parsing %s: %v", baselinePath, err)
	}
	return m
}

func checkBaseline(t *testing.T, results []auditResult) {
	t.Helper()
	base := loadBaseline(t)
	if *updateBaseline {
		next := map[string]baselineEntry{}
		for _, res := range results {
			got := gatedFindings(res)
			if len(got) == 0 {
				continue
			}
			e, ok := base[res.ID]
			if !ok {
				e = baselineEntry{Status: "new", Reason: "classify: gap (with phase), design or tooling"}
			}
			e.Findings = got
			next[res.ID] = e
		}
		b, _ := json.MarshalIndent(next, "", "  ")
		if err := os.WriteFile(baselinePath, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s: %d entries", baselinePath, len(next))
		base = next
	}
	ids := map[string]bool{}
	for _, res := range results {
		ids[res.ID] = true
		got := gatedFindings(res)
		e, ok := base[res.ID]
		switch {
		case !ok && len(got) > 0:
			t.Errorf("%s %s: new findings; fix them, or classify them in %s:\n  %s", res.ID, res.Title, baselinePath, strings.Join(got, "\n  "))
		case ok && len(got) == 0:
			t.Errorf("%s %s: no findings now, but the baseline expects %d (%s); remove the entry", res.ID, res.Title, len(e.Findings), e.Status)
		case ok && strings.Join(got, "\n") != strings.Join(e.Findings, "\n"):
			t.Errorf("%s %s: findings changed.\n got:  %s\n want: %s", res.ID, res.Title, strings.Join(got, "\n        "), strings.Join(e.Findings, "\n        "))
		case ok && e.Status == "new":
			t.Errorf("%s: classify the baseline entry (status \"new\")", res.ID)
		case ok && e.Status == "gap" && e.Phase == "":
			t.Errorf("%s: a known gap needs the phase that fixes it", res.ID)
		}
	}
	for id := range base {
		if !ids[id] {
			t.Errorf("%s: in the baseline but no such scenario", id)
		}
	}
}

// TestAudit runs every scenario, then checks the findings against the
// baseline.
func TestAudit(t *testing.T) {
	out := os.Getenv("AUDIT_OUT")
	if out != "" {
		_ = os.MkdirAll(filepath.Join(out, "specs"), 0o755)
	}
	var all []scenario
	for _, group := range [][]scenario{typeScenarios(), paramScenarios(), bodyScenarios(), responseScenarios(), routingScenarios(), securityScenarios(), metadataScenarios(), servingScenarios()} {
		all = append(all, group...)
	}
	seen := map[string]bool{}
	for _, sc := range all {
		if seen[sc.id] {
			t.Fatalf("duplicate scenario id %s", sc.id)
		}
		seen[sc.id] = true
		res := runScenario(t, sc, out)
		auditMu.Lock()
		auditResults = append(auditResults, res)
		auditMu.Unlock()
		n := len(res.Valid) + len(res.Expect) + len(res.Conformance)
		if n > 0 {
			t.Logf("%s %s: %d findings", sc.id, sc.title, n)
		}
	}
	if out != "" {
		b, _ := json.MarshalIndent(auditResults, "", "  ")
		_ = os.WriteFile(filepath.Join(out, "results.json"), b, 0o644)
	}
	t.Logf("%d scenarios", len(all))
	checkBaseline(t, auditResults)
}

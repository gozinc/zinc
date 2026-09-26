// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package benchmarks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Every scenario proves, before timing, that its frameworks give the same
// answer to the same requests. Without it a framework can skip work the
// others do (an unbound query, a missing header) and look faster.

// proofRequests is how many of a scenario's requests are checked.
const proofRequests = 32

// statusOnlyProof names the scenarios that deliberately keep each framework's
// default error response, whose body and headers differ by design.
var statusOnlyProof = map[string]bool{
	"NotFound":           true,
	"StaticFileNotFound": true,
}

// proofResponse is the part of a response every framework must agree on.
type proofResponse struct {
	Status      int
	MediaType   string
	Allow       []string
	Body        string
	JSON        any
	SinkInt     int
	SinkString  string
	isJSON      bool
	statusOnly  bool
	requestDesc string
}

func captureProof(handler http.Handler, req *http.Request, statusOnly bool) proofResponse {
	rec := httptest.NewRecorder()
	// Handlers record the parameters they read in the sinks, so comparing
	// sinks proves every framework did the same parameter work.
	benchmarkSinkInt, benchmarkSinkString = 0, ""
	handler.ServeHTTP(rec, req)
	p := proofResponse{Status: rec.Code, SinkInt: benchmarkSinkInt, SinkString: benchmarkSinkString,
		statusOnly: statusOnly, requestDesc: req.Method + " " + req.URL.RequestURI()}
	if statusOnly {
		return p
	}
	p.MediaType, _, _ = mime.ParseMediaType(rec.Header().Get("Content-Type"))
	for _, v := range rec.Header().Values("Allow") {
		for _, m := range strings.Split(v, ",") {
			// Automatic HEAD and OPTIONS answers differ between frameworks.
			if m = strings.TrimSpace(m); m != "" && m != http.MethodHead && m != http.MethodOptions {
				p.Allow = append(p.Allow, m)
			}
		}
	}
	slices.Sort(p.Allow)
	body := rec.Body.Bytes()
	if p.MediaType == "application/json" && len(bytes.TrimSpace(body)) > 0 {
		if json.Unmarshal(body, &p.JSON) == nil {
			p.isJSON = true
			return p
		}
	}
	p.Body = string(body)
	return p
}

func (p proofResponse) differs(q proofResponse) string {
	switch {
	case p.Status != q.Status:
		return fmt.Sprintf("status %d, want %d", q.Status, p.Status)
	case p.statusOnly:
		return ""
	case p.MediaType != q.MediaType:
		return fmt.Sprintf("Content-Type %q, want %q", q.MediaType, p.MediaType)
	case !slices.Equal(p.Allow, q.Allow):
		return fmt.Sprintf("Allow %v, want %v", q.Allow, p.Allow)
	case p.isJSON != q.isJSON:
		return fmt.Sprintf("body %q%v, want %q%v", q.Body, q.JSON, p.Body, p.JSON)
	case p.isJSON && !reflect.DeepEqual(p.JSON, q.JSON):
		return fmt.Sprintf("JSON body %v, want %v", q.JSON, p.JSON)
	case !p.isJSON && p.Body != q.Body:
		return fmt.Sprintf("body %q, want %q", q.Body, p.Body)
	case p.SinkInt != q.SinkInt || p.SinkString != q.SinkString:
		return fmt.Sprintf("parameters read %d %q, want %d %q", q.SinkInt, q.SinkString, p.SinkInt, p.SinkString)
	}
	return ""
}

// proveCases checks every case against the first (Zinc) on up to
// proofRequests of the scenario's requests. next(i) returns the i-th request,
// ready to serve (a prepared request resets its body).
func proveCases(b *testing.B, cases []benchmarkCase, n int, next func(i int) *http.Request) {
	b.Helper()
	if len(cases) < 2 || n == 0 {
		return
	}
	top, _, _ := strings.Cut(strings.TrimPrefix(b.Name(), "Benchmark"), "/")
	statusOnly := statusOnlyProof[top]
	handlers := make([]http.Handler, len(cases))
	for i, bc := range cases {
		handlers[i] = bc.build()
	}
	for i := 0; i < min(n, proofRequests); i++ {
		want := captureProof(handlers[0], next(i), statusOnly)
		for c := 1; c < len(cases); c++ {
			got := captureProof(handlers[c], next(i), statusOnly)
			if diff := want.differs(got); diff != "" {
				b.Fatalf("%s disagrees with %s on %s: %s", cases[c].name, cases[0].name, want.requestDesc, diff)
			}
		}
	}
}

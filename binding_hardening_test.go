package zinc_test

import (
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

type hardeningValidator struct{ calls *int }

func (v hardeningValidator) Validate(any) error { *v.calls++; return nil }

type failingDecodeCodec struct{}

func (failingDecodeCodec) Encode(io.Writer, any, string) error { return nil }
func (failingDecodeCodec) Decode(io.Reader, any) error         { return errors.New("internal codec failure") }

func TestBindingAllSourceOrderAcrossStructuredFormats(t *testing.T) {
	for _, tt := range []struct{ kind, body string }{{"application/json", `{"name":"body"}`}, {"application/xml", `<input><name>body</name></input>`}, {"application/yaml", "name: body\n"}, {"application/toml", "name = 'body'\n"}} {
		t.Run(tt.kind, func(t *testing.T) {
			calls := 0
			cfg := zinc.DefaultConfig
			cfg.Validator = hardeningValidator{&calls}
			app := zinc.NewWithConfig(cfg)
			app.Post("/items/{id}", func(c *zinc.Context) error {
				var v struct {
					ID   string `path:"id"`
					Name string `query:"name" json:"name" xml:"name" yaml:"name" toml:"name"`
				}
				if err := c.Bind().All(&v); err != nil {
					return err
				}
				return c.Send(v.ID + "|" + v.Name)
			})
			r := httptest.NewRequest("POST", "/items/42?name=query", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", tt.kind)
			w := httptest.NewRecorder()
			app.ServeHTTP(w, r)
			if w.Code != 200 || w.Body.String() != "42|body" || calls != 1 {
				t.Fatalf("%d %q validation=%d", w.Code, w.Body.String(), calls)
			}
		})
	}
}

func TestBindingErrorHTTPPolicy(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		codec      bool
		bind       func(*zinc.Context) error
	}{
		{"malformed-json", `{"name":`, 400, false, func(c *zinc.Context) error { var v map[string]any; return c.Bind().JSON(&v) }},
		{"json-type", `{"age":"bad"}`, 400, false, func(c *zinc.Context) error { var v struct{ Age int }; return c.Bind().JSON(&v) }},
		{"invalid-json-destination", `{}`, 500, false, func(c *zinc.Context) error { return c.Bind().JSON(42) }},
		{"internal-codec", `{}`, 500, true, func(c *zinc.Context) error { var v map[string]any; return c.Bind().JSON(&v) }},
		{"malformed-xml", `<broken`, 400, false, func(c *zinc.Context) error {
			var v struct{ Name string }
			err := c.Bind().XML(&v)
			var be *zinc.BindError
			if !errors.As(err, &be) || be.Source != "body" {
				t.Errorf("missing XML binding context: %v", err)
			}
			return err
		}},
		{"empty-json", ``, 400, false, func(c *zinc.Context) error { var v map[string]any; return c.Bind().JSON(&v) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := zinc.DefaultConfig
			if tt.codec {
				cfg.JSONCodec = failingDecodeCodec{}
			}
			app := zinc.NewWithConfig(cfg)
			app.Post("/", tt.bind)
			w := httptest.NewRecorder()
			app.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(tt.body)))
			if w.Code != tt.status {
				t.Fatalf("%d %q", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "codec") || strings.Contains(w.Body.String(), "offset") {
				t.Fatal("decoder internals exposed")
			}
		})
	}
}

type strictID string

func (v *strictID) UnmarshalText(text []byte) error {
	if !strings.HasPrefix(string(text), "id-") {
		return errors.New("invalid ID")
	}
	*v = strictID(text)
	return nil
}

func TestScalarBindingHooksAndOptionalPointers(t *testing.T) {
	app := zinc.New()
	app.Get("/items/{id}", func(c *zinc.Context) error {
		var v struct {
			ID     strictID  `path:"id"`
			Limit  *int      `query:"limit"`
			Other  *int      `query:"other"`
			Custom *strictID `query:"custom"`
		}
		if err := c.Bind().All(&v); err != nil {
			return err
		}
		if v.ID != "id-42" || v.Limit == nil || *v.Limit != 0 || v.Other != nil || v.Custom == nil || *v.Custom != "id-7" {
			t.Errorf("bad binding: %+v", v)
		}
		return c.Send("ok")
	})
	for _, tt := range []struct {
		target string
		code   int
	}{{"/items/id-42?limit=0&custom=id-7", 200}, {"/items/invalid?limit=0&custom=id-7", 400}, {"/items/id-42?limit=wrong&custom=id-7", 400}, {"/items/id-42?limit=0&custom=wrong", 400}} {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", tt.target, nil))
		if w.Code != tt.code {
			t.Fatalf("%s: %d %q", tt.target, w.Code, w.Body.String())
		}
	}
}

func TestTextUnmarshalerAcrossRequestSources(t *testing.T) {
	app := zinc.New()
	app.Post("/", func(c *zinc.Context) error {
		var h struct {
			ID strictID `header:"X-ID"`
		}
		if err := c.Bind().Header(&h); err != nil {
			return err
		}
		var f struct {
			ID strictID `form:"id"`
		}
		if err := c.Bind().Form(&f); err != nil {
			return err
		}
		return c.Send(string(h.ID) + "|" + string(f.ID))
	})
	r := httptest.NewRequest("POST", "/", strings.NewReader("id=id-form"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-ID", "id-header")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != "id-header|id-form" {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
}

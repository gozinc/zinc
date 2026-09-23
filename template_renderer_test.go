// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"errors"
	"html/template"
	"io"
	"net/http"
	"strings"
	"testing"
	texttemplate "text/template"
)

func TestTemplateRendererHelpers(t *testing.T) {
	t.Run("renders with html templates", func(t *testing.T) {
		tmpl := template.Must(template.New("home").Parse("Hello, {{.Name}}!"))
		app := NewWithConfig(Config{
			Renderer: NewHTMLTemplateRenderer(tmpl),
		})
		app.Get("/home", func(c *Context) error {
			return c.Render("home", Map{"Name": "Zinc"})
		})

		resp := performRequest(t, app, http.MethodGet, "/home", nil, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("status=%d", resp.Code)
		}
		if body := strings.TrimSpace(resp.Body.String()); body != "Hello, Zinc!" {
			t.Fatalf("body=%q", body)
		}
	})

	t.Run("keeps status chaining behavior", func(t *testing.T) {
		tmpl := template.Must(template.New("created").Parse("created"))
		app := NewWithConfig(Config{
			Renderer: NewTemplateRenderer(tmpl),
		})
		app.Get("/created", func(c *Context) error {
			return c.Status(http.StatusCreated).Render("created", nil)
		})

		resp := performRequest(t, app, http.MethodGet, "/created", nil, nil)
		if resp.Code != http.StatusCreated {
			t.Fatalf("status=%d", resp.Code)
		}
		if body := strings.TrimSpace(resp.Body.String()); body != "created" {
			t.Fatalf("body=%q", body)
		}
	})

	t.Run("supports suffix fallback", func(t *testing.T) {
		tmpl := template.Must(template.New("dashboard.html").Parse("Dashboard"))
		app := NewWithConfig(Config{
			Renderer: NewHTMLTemplateRenderer(tmpl, WithTemplateSuffixes("html", ".tmpl")),
		})
		app.Get("/dashboard", func(c *Context) error {
			return c.Render("dashboard", nil)
		})

		resp := performRequest(t, app, http.MethodGet, "/dashboard", nil, nil)
		if body := strings.TrimSpace(resp.Body.String()); body != "Dashboard" {
			t.Fatalf("body=%q", body)
		}
	})

	t.Run("renders with text templates", func(t *testing.T) {
		tmpl := texttemplate.Must(texttemplate.New("plain").Parse("Hello, {{.Name}}!"))
		app := NewWithConfig(Config{
			Renderer: NewTextTemplateRenderer(tmpl),
		})
		app.Get("/plain", func(c *Context) error {
			return c.Render("plain", Map{"Name": "Zinc"})
		})

		resp := performRequest(t, app, http.MethodGet, "/plain", nil, nil)
		if body := strings.TrimSpace(resp.Body.String()); body != "Hello, Zinc!" {
			t.Fatalf("body=%q", body)
		}
	})

	t.Run("returns not found sentinel when template is absent", func(t *testing.T) {
		tmpl := template.Must(template.New("home.html").Parse("home"))
		renderer := NewHTMLTemplateRenderer(tmpl, WithTemplateSuffixes(".html"))

		err := renderer.Render(io.Discard, "missing", nil, nil)
		if !errors.Is(err, ErrTemplateNotFound) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("requires engine and template name", func(t *testing.T) {
		renderer := NewTemplateRenderer(nil)
		if err := renderer.Render(io.Discard, "home", nil, nil); !errors.Is(err, ErrTemplateEngineNotConfigured) {
			t.Fatalf("err=%v", err)
		}

		renderer = NewTemplateRenderer(template.Must(template.New("home").Parse("ok")))
		if err := renderer.Render(io.Discard, " ", nil, nil); !errors.Is(err, ErrTemplateNameRequired) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("tries fallback candidates for custom engines", func(t *testing.T) {
		engine := &templateEngineStub{
			match: "home.tmpl",
		}
		renderer := NewTemplateRenderer(engine, WithTemplateSuffixes(".html", ".tmpl"))

		var out strings.Builder
		mustDo(t, renderer.Render(&out, "home", nil, nil))
		if out.String() != "ok" {
			t.Fatalf("out=%q", out.String())
		}
		calls := strings.Join(engine.calls, ",")
		if calls != "home,home.html,home.tmpl" {
			t.Fatalf("calls=%q", calls)
		}
	})
}

type templateEngineStub struct {
	calls []string
	match string
}

func (s *templateEngineStub) ExecuteTemplate(w io.Writer, name string, _ any) error {
	s.calls = append(s.calls, name)
	if name != s.match {
		return errors.New("missing template")
	}
	_, _ = io.WriteString(w, "ok")
	return nil
}

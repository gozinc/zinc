// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

type rendererStub struct{}

type xmlPayload struct {
	XMLName xml.Name `xml:"response"`
	Value   string   `xml:"value"`
}

func (rendererStub) Render(w io.Writer, name string, data any, c *Context) error {
	_, err := io.WriteString(w, name+":ok")
	return err
}

func TestResponseHelpers(t *testing.T) {
	t.Run("headers cookies and content helpers", func(t *testing.T) {
		app := New()
		app.Get("/headers", func(c *Context) error {
			c.SetHeader("X-One", "1").AppendHeader("X-Many", "a", "b").Type("json").Location("/next").Vary("Origin", "Accept")
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie(&http.Cookie{Name: "session", Value: "abc", Path: "/"})
			c.ClearCookie("stale")
			return c.String("ok")
		})
		resp := performRequest(t, app, http.MethodGet, "/headers", nil, nil)
		if resp.Header().Get("X-One") != "1" || resp.Header().Get(HeaderLocation) != "/next" {
			t.Fatal("header helpers failed")
		}
		if got := resp.Header().Values("X-Many"); len(got) != 2 {
			t.Fatalf("append header=%v", got)
		}
		if ct := resp.Header().Get(HeaderContentType); ct != "application/json" {
			t.Fatalf("content type=%q", ct)
		}
		if vary := resp.Header().Values(HeaderVary); len(vary) != 2 {
			t.Fatalf("vary=%v", vary)
		}
		cookies := resp.Result().Cookies()
		if len(cookies) < 2 {
			t.Fatalf("cookies=%v", cookies)
		}
		if cookies[0].SameSite != http.SameSiteLaxMode || cookies[1].SameSite != http.SameSiteLaxMode {
			t.Fatalf("cookie same site=%v %v", cookies[0].SameSite, cookies[1].SameSite)
		}
	})

	t.Run("data encoders and streams", func(t *testing.T) {
		app := New()
		app.Get("/data", func(c *Context) error { return c.Data("application/custom", []byte("data")) })
		app.Get("/blob", func(c *Context) error { return c.Blob(http.StatusCreated, "application/custom", []byte("blob")) })
		app.Get("/json-blob", func(c *Context) error { return c.JSONBlob(http.StatusAccepted, []byte(`{"ok":true}`)) })
		app.Get("/xml-blob", func(c *Context) error { return c.XMLBlob(http.StatusAccepted, []byte(`<ok>true</ok>`)) })
		app.Get("/html-blob", func(c *Context) error { return c.HTMLBlob(http.StatusAccepted, []byte(`<p>ok</p>`)) })
		app.Get("/json", func(c *Context) error { return c.JSONPretty(Map{"ok": true}, "  ") })
		app.Get("/xml", func(c *Context) error { return c.XML(xmlPayload{Value: "x"}) })
		app.Get("/yaml", func(c *Context) error { return c.YAML(Map{"ok": true}) })
		app.Get("/toml", func(c *Context) error { return c.TOML(Map{"ok": true}) })
		app.Get("/html", func(c *Context) error { return c.HTML("<p>x</p>") })
		app.Get("/stream", func(c *Context) error { return c.Stream("text/plain", strings.NewReader("stream")) })
		app.Get("/send-nil", func(c *Context) error { return c.Send(nil) })
		app.Get("/send-bytes", func(c *Context) error { return c.Send([]byte("bytes")) })

		cases := map[string]string{
			"/data":       "data",
			"/blob":       "blob",
			"/json-blob":  `{"ok":true}`,
			"/xml-blob":   `<ok>true</ok>`,
			"/html-blob":  `<p>ok</p>`,
			"/xml":        "<value>x</value>",
			"/yaml":       "ok: true",
			"/toml":       "ok = true",
			"/html":       "<p>x</p>",
			"/stream":     "stream",
			"/send-bytes": "bytes",
		}
		for path, expected := range cases {
			resp := performRequest(t, app, http.MethodGet, path, nil, nil)
			if !strings.Contains(resp.Body.String(), expected) {
				t.Fatalf("%s body=%q", path, resp.Body.String())
			}
		}
		jsonResp := performRequest(t, app, http.MethodGet, "/json", nil, nil)
		if !strings.Contains(jsonResp.Body.String(), "\n") {
			t.Fatalf("json pretty=%q", jsonResp.Body.String())
		}
		nilResp := performRequest(t, app, http.MethodGet, "/send-nil", nil, nil)
		if nilResp.Body.String() != "null" {
			t.Fatalf("nil body=%q", nilResp.Body.String())
		}
		blob := performRequest(t, app, http.MethodGet, "/blob", nil, nil)
		if blob.Code != http.StatusCreated || blob.Header().Get(HeaderContentType) != "application/custom" {
			t.Fatalf("blob status=%d content-type=%q", blob.Code, blob.Header().Get(HeaderContentType))
		}
		jsonBlob := performRequest(t, app, http.MethodGet, "/json-blob", nil, nil)
		if jsonBlob.Code != http.StatusAccepted || jsonBlob.Header().Get(HeaderContentType) != jsonType {
			t.Fatalf("json blob status=%d content-type=%q", jsonBlob.Code, jsonBlob.Header().Get(HeaderContentType))
		}
	})

	t.Run("sse", func(t *testing.T) {
		app := New()
		app.Get("/events", func(c *Context) error {
			c.Status(http.StatusCreated)
			if err := c.SSE(SSEvent{
				Event: "message",
				ID:    "1",
				Retry: 2 * time.Second,
				Data:  Map{"ok": true},
			}); err != nil {
				return err
			}
			return c.SSE(SSEvent{Data: "line 1\nline 2"})
		})

		resp := performRequest(t, app, http.MethodGet, "/events", nil, nil)
		if resp.Code != http.StatusCreated {
			t.Fatalf("sse status=%d", resp.Code)
		}
		if ct := resp.Header().Get(HeaderContentType); ct != eventStream {
			t.Fatalf("sse content type=%q", ct)
		}
		expected := "event: message\nid: 1\nretry: 2000\ndata: {\"ok\":true}\n\ndata: line 1\ndata: line 2\n\n"
		if resp.Body.String() != expected {
			t.Fatalf("sse body=%q", resp.Body.String())
		}
	})

	t.Run("content negotiation", func(t *testing.T) {
		app := New()
		app.Get("/negotiate", func(c *Context) error {
			return c.Negotiate(http.StatusAccepted, map[string]any{
				"application/json": Map{"ok": true},
				"text/plain":       "plain",
			})
		})

		jsonResp := performRequest(t, app, http.MethodGet, "/negotiate", nil, map[string]string{
			HeaderAccept: "text/plain;q=0.1, application/json;q=0.9",
		})
		if jsonResp.Code != http.StatusAccepted || jsonResp.Header().Get(HeaderContentType) != jsonType || !strings.Contains(jsonResp.Body.String(), `"ok":true`) {
			t.Fatalf("json negotiation status=%d content-type=%q body=%q", jsonResp.Code, jsonResp.Header().Get(HeaderContentType), jsonResp.Body.String())
		}

		textResp := performRequest(t, app, http.MethodGet, "/negotiate", nil, map[string]string{
			HeaderAccept: "text/*",
		})
		if textResp.Code != http.StatusAccepted || textResp.Body.String() != "plain" {
			t.Fatalf("text negotiation status=%d body=%q", textResp.Code, textResp.Body.String())
		}

		notAcceptable := performRequest(t, app, http.MethodGet, "/negotiate", nil, map[string]string{
			HeaderAccept: "image/png",
		})
		if notAcceptable.Code != http.StatusNotAcceptable {
			t.Fatalf("not acceptable status=%d", notAcceptable.Code)
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(HeaderAccept, "text/html;q=0.9, application/json;q=0.1")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()
		if got := ctx.Accepts("application/json", "text/html"); got != "text/html" {
			t.Fatalf("accepts=%q", got)
		}
	})

	t.Run("no content redirect render and repeated write", func(t *testing.T) {
		app := NewWithConfig(Config{Renderer: rendererStub{}})
		app.Get("/nocontent", func(c *Context) error { return c.NoContent() })
		app.Get("/redirect", func(c *Context) error { return c.Redirect(http.StatusMovedPermanently, "/to") })
		app.Get("/render", func(c *Context) error { return c.Render("home", nil) })
		app.Get("/double", func(c *Context) error {
			mustDo(t, c.String("once"))
			return c.String("twice")
		})

		noContent := performRequest(t, app, http.MethodGet, "/nocontent", nil, nil)
		if noContent.Code != http.StatusNoContent || noContent.Body.Len() != 0 {
			t.Fatalf("nocontent=%d %q", noContent.Code, noContent.Body.String())
		}
		redirect := performRequest(t, app, http.MethodGet, "/redirect", nil, nil)
		if redirect.Code != http.StatusMovedPermanently || redirect.Header().Get(HeaderLocation) != "/to" {
			t.Fatalf("redirect=%d %q", redirect.Code, redirect.Header().Get(HeaderLocation))
		}
		render := performRequest(t, app, http.MethodGet, "/render", nil, nil)
		if render.Body.String() != "home:ok" {
			t.Fatalf("render=%q", render.Body.String())
		}
		double := performRequest(t, app, http.MethodGet, "/double", nil, nil)
		if double.Code != http.StatusOK || double.Body.String() != "once" {
			t.Fatalf("double=%d %q", double.Code, double.Body.String())
		}
	})

	t.Run("file helpers", func(t *testing.T) {
		filesystem := fstest.MapFS{"hello.txt": &fstest.MapFile{Data: []byte("world")}}
		app := New()
		app.Get("/filefs", func(c *Context) error { return c.FileFS("hello.txt", filesystem) })
		app.Get("/attach", func(c *Context) error { return c.Attachment("testdata.txt", "download.txt") })

		fileResp := performRequest(t, app, http.MethodGet, "/filefs", nil, nil)
		if fileResp.Body.String() != "world" {
			t.Fatalf("filefs body=%q", fileResp.Body.String())
		}

		dir := t.TempDir()
		path := filepath.Join(dir, "testdata.txt")
		mustDo(t, os.WriteFile(path, []byte("attachment"), 0o644))
		app.Get("/file", func(c *Context) error { return c.File(path) })
		app.Get("/download", func(c *Context) error { return c.Download(path) })
		app.Get("/download-named", func(c *Context) error { return c.Download(path, "custom-name.txt") })
		app.Get("/inline", func(c *Context) error { return c.Inline(path, "inline-name.txt") })
		attachApp := New()
		attachApp.Get("/attach", func(c *Context) error { return c.Attachment(path, "download.txt") })

		file := performRequest(t, app, http.MethodGet, "/file", nil, nil)
		if file.Body.String() != "attachment" {
			t.Fatalf("file body=%q", file.Body.String())
		}
		download := performRequest(t, app, http.MethodGet, "/download", nil, nil)
		if disposition := download.Header().Get(HeaderContentDisposition); disposition == "" {
			t.Fatal("download disposition missing")
		}
		downloadNamed := performRequest(t, app, http.MethodGet, "/download-named", nil, nil)
		if disposition := downloadNamed.Header().Get(HeaderContentDisposition); !strings.Contains(disposition, "custom-name.txt") {
			t.Fatalf("named download disposition=%q", disposition)
		}
		attach := performRequest(t, attachApp, http.MethodGet, "/attach", nil, nil)
		if disposition := attach.Header().Get(HeaderContentDisposition); !strings.Contains(disposition, "download.txt") {
			t.Fatalf("attachment disposition=%q", disposition)
		}
		inline := performRequest(t, app, http.MethodGet, "/inline", nil, nil)
		if disposition := inline.Header().Get(HeaderContentDisposition); !strings.Contains(disposition, "inline") || !strings.Contains(disposition, "inline-name.txt") {
			t.Fatalf("inline disposition=%q", disposition)
		}
	})

	t.Run("head semantics", func(t *testing.T) {
		app := New()
		app.Get("/head", func(c *Context) error { return c.JSON(Map{"ok": true}) })
		resp := performRequest(t, app, http.MethodHead, "/head", nil, nil)
		if resp.Body.Len() != 0 {
			t.Fatalf("head body=%q", resp.Body.String())
		}
	})
}

func TestDirectResponseErrors(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx, _ := newRecorderContext(t, req)
	defer ctx.release()
	mustDo(t, ctx.String("once"))
	if err := ctx.String("twice"); !errors.Is(err, ErrResponseAlreadySent) {
		t.Fatalf("err=%v", err)
	}
}

func TestResponseInternalHelpers(t *testing.T) {
	if bodyAllowed(http.MethodGet, http.StatusContinue) {
		t.Fatal("1xx status must not allow response body")
	}
	if bodyAllowed(http.MethodHead, http.StatusOK) {
		t.Fatal("HEAD must not allow response body")
	}
	if bodyAllowed(http.MethodGet, http.StatusNoContent) {
		t.Fatal("204 must not allow response body")
	}
	if !bodyAllowed(http.MethodGet, http.StatusOK) {
		t.Fatal("200 GET should allow response body")
	}

	var c Context
	if got := c.responseStatus(); got != http.StatusOK {
		t.Fatalf("status=%d", got)
	}
	c.status = http.StatusAccepted
	if got := c.responseStatus(); got != http.StatusAccepted {
		t.Fatalf("status=%d", got)
	}

	ctx, rec := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctx.release()
	ctx.Type("")
	if got := rec.Header().Get(HeaderContentType); got != "" {
		t.Fatalf("content type=%q", got)
	}

	ranges := parseAcceptHeader("text/*;q=0.4, application/json;q=bad, */*;q=0, application/xml;q=0.8, broken")
	if len(ranges) != 3 {
		t.Fatalf("ranges=%v", ranges)
	}
	if ranges[0].mediaType != "broken" || ranges[1].mediaType != "application/xml" || ranges[2].mediaType != "text/*" {
		t.Fatalf("ranges=%v", ranges)
	}
	if acceptSpecificity("*/*") != 0 || acceptSpecificity("text/*") != 1 || acceptSpecificity("application/json") != 2 {
		t.Fatal("accept specificity failed")
	}
	if acceptMatches("text/*", "application/json") {
		t.Fatal("text wildcard should not match application/json")
	}
	if acceptMatches("broken", "text/plain") {
		t.Fatal("invalid accept range should not match")
	}
}

func TestWriteJSONNilAndXMLNil(t *testing.T) {
	jsonCtx, jsonRec := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer jsonCtx.release()
	mustDo(t, jsonCtx.writeJSON(nil, ""))
	if jsonRec.Body.String() != "null" {
		t.Fatalf("json body=%q", jsonRec.Body.String())
	}

	xmlCtx, xmlRec := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer xmlCtx.release()
	mustDo(t, xmlCtx.XML(nil))
	if xmlRec.Body.String() != "null" {
		t.Fatalf("xml body=%q", xmlRec.Body.String())
	}
}

func TestWriteNegotiatedContentTypes(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		value       any
		wantType    string
		wantBody    string
	}{
		{
			name:        "json bytes",
			contentType: "application/json; charset=utf-8",
			value:       []byte(`{"ok":true}`),
			wantType:    jsonType,
			wantBody:    `{"ok":true}`,
		},
		{
			name:        "json value",
			contentType: "application/json",
			value:       Map{"ok": true},
			wantType:    jsonType,
			wantBody:    "{\"ok\":true}\n",
		},
		{
			name:        "xml bytes",
			contentType: "text/xml",
			value:       []byte(`<ok>true</ok>`),
			wantType:    xmlType,
			wantBody:    `<ok>true</ok>`,
		},
		{
			name:        "xml value",
			contentType: "application/xml",
			value:       xmlPayload{Value: "x"},
			wantType:    xmlType,
			wantBody:    "<response><value>x</value></response>",
		},
		{
			name:        "yaml bytes",
			contentType: "application/x-yaml",
			value:       []byte("ok: true\n"),
			wantType:    yamlType,
			wantBody:    "ok: true\n",
		},
		{
			name:        "yaml value",
			contentType: "text/yaml",
			value:       Map{"ok": true},
			wantType:    yamlType,
			wantBody:    "ok: true\n",
		},
		{
			name:        "toml bytes",
			contentType: "application/toml",
			value:       []byte("ok = true\n"),
			wantType:    tomlType,
			wantBody:    "ok = true\n",
		},
		{
			name:        "toml value",
			contentType: "application/toml",
			value:       Map{"ok": true},
			wantType:    tomlType,
			wantBody:    "ok = true\n",
		},
		{
			name:        "html bytes",
			contentType: "text/html",
			value:       []byte("<p>bytes</p>"),
			wantType:    htmlType,
			wantBody:    "<p>bytes</p>",
		},
		{
			name:        "html string",
			contentType: "text/html",
			value:       "<p>ok</p>",
			wantType:    htmlType,
			wantBody:    "<p>ok</p>",
		},
		{
			name:        "plain string",
			contentType: "text/plain",
			value:       "plain",
			wantType:    plainText,
			wantBody:    "plain",
		},
		{
			name:        "plain value",
			contentType: "text/plain",
			value:       42,
			wantType:    plainText,
			wantBody:    "42",
		},
		{
			name:        "default bytes",
			contentType: "application/custom",
			value:       []byte("custom-bytes"),
			wantType:    "application/custom",
			wantBody:    "custom-bytes",
		},
		{
			name:        "default reader",
			contentType: "application/octet-stream",
			value:       strings.NewReader("streamed"),
			wantType:    "application/octet-stream",
			wantBody:    "streamed",
		},
		{
			name:        "default string",
			contentType: "application/custom",
			value:       "custom",
			wantType:    "application/custom",
			wantBody:    "custom",
		},
		{
			name:        "default value",
			contentType: "application/custom",
			value:       42,
			wantType:    "application/custom",
			wantBody:    "42",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, rec := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
			defer ctx.release()

			mustDo(t, ctx.writeNegotiated(tc.contentType, tc.value))
			if got := rec.Header().Get(HeaderContentType); got != tc.wantType {
				t.Fatalf("content type=%q want %q", got, tc.wantType)
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Fatalf("body=%q want %q", got, tc.wantBody)
			}
		})
	}
}

func TestRedirectAndRenderErrorBranches(t *testing.T) {
	ctx, rec := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctx.release()
	mustDo(t, ctx.Redirect(0, "/next"))
	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Header().Get(HeaderLocation) != "/next" {
		t.Fatalf("location=%q", rec.Header().Get(HeaderLocation))
	}
	if err := ctx.Redirect(http.StatusTemporaryRedirect, "/again"); !errors.Is(err, ErrResponseAlreadySent) {
		t.Fatalf("err=%v", err)
	}

	noAppCtx := &Context{}
	if err := noAppCtx.Render("home", nil); err == nil || !strings.Contains(err.Error(), "renderer is not configured") {
		t.Fatalf("err=%v", err)
	}

	noRendererCtx, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer noRendererCtx.release()
	noRendererCtx.app = New()
	if err := noRendererCtx.Render("home", nil); err == nil || !strings.Contains(err.Error(), "renderer is not configured") {
		t.Fatalf("err=%v", err)
	}

	renderErrCtx, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer renderErrCtx.release()
	renderErrCtx.app = NewWithConfig(Config{
		Renderer: rendererErrorStub{err: errors.New("render failed")},
	})
	if err := renderErrCtx.Render("home", nil); err == nil || !strings.Contains(err.Error(), "render failed") {
		t.Fatalf("err=%v", err)
	}
}

func TestWriteResponseAndPrepareResponseBranches(t *testing.T) {
	ctx, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctx.release()
	ctx.written = true
	if err := ctx.writeResponse("", nil); !errors.Is(err, ErrResponseAlreadySent) {
		t.Fatalf("err=%v", err)
	}

	noBodyCtx, noBodyRec := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer noBodyCtx.release()
	noBodyCtx.Status(http.StatusNoContent)
	called := false
	mustDo(t, noBodyCtx.writeResponse("text/plain", func() error {
		called = true
		return nil
	}))
	if called {
		t.Fatal("writeBody should not run for 204 responses")
	}
	if noBodyRec.Code != http.StatusNoContent {
		t.Fatalf("status=%d", noBodyRec.Code)
	}

	headCtx, headRec := newRecorderContext(t, httptest.NewRequest(http.MethodHead, "/", nil))
	defer headCtx.release()
	mustDo(t, headCtx.writeResponse("text/plain", func() error {
		t.Fatal("writeBody should not run for HEAD")
		return nil
	}))
	if headRec.Body.Len() != 0 {
		t.Fatalf("head body=%q", headRec.Body.String())
	}

	prepareErrCtx, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer prepareErrCtx.release()
	prepareErrCtx.written = true
	if _, _, err := prepareErrCtx.prepareResponse(jsonType); !errors.Is(err, ErrResponseAlreadySent) {
		t.Fatalf("err=%v", err)
	}

	prepareHeaderCtx, prepareHeaderRec := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer prepareHeaderCtx.release()
	prepareHeaderRec.Header().Set(contentType, "application/custom")
	_, writeBody, err := prepareHeaderCtx.prepareResponse(jsonType)
	mustDo(t, err)
	if !writeBody {
		t.Fatal("writeBody should be true for 200 GET")
	}
	if got := prepareHeaderRec.Header().Get(contentType); got != "application/custom" {
		t.Fatalf("content type=%q", got)
	}
}

func TestServeFileErrorAndFallbackBranches(t *testing.T) {
	ctxWritten, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctxWritten.release()
	ctxWritten.written = true
	if err := ctxWritten.serveFile("anything", fstest.MapFS{}, ""); !errors.Is(err, ErrResponseAlreadySent) {
		t.Fatalf("err=%v", err)
	}

	ctxOpenErr, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctxOpenErr.release()
	if err := ctxOpenErr.serveFile("missing.txt", fstest.MapFS{}, ""); err == nil {
		t.Fatal("expected open error")
	}

	ctxStatErr, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctxStatErr.release()
	err := ctxStatErr.serveFile("x", openFS{open: func(string) (fs.File, error) {
		return &statErrFile{}, nil
	}}, "")
	if err == nil || !strings.Contains(err.Error(), "stat failed") {
		t.Fatalf("err=%v", err)
	}

	ctxDir, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctxDir.release()
	err = ctxDir.serveFile("x", openFS{open: func(string) (fs.File, error) {
		return &memoryFile{
			info: fileInfoStub{name: "x", dir: true},
		}, nil
	}}, "")
	if !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("err=%v", err)
	}

	ctxReadErr, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctxReadErr.release()
	err = ctxReadErr.serveFile("x", openFS{open: func(string) (fs.File, error) {
		return &readErrFile{
			info: fileInfoStub{name: "x.txt"},
			err:  errors.New("read failed"),
		}, nil
	}}, "")
	if err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("err=%v", err)
	}

	ctxFallback, recFallback := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctxFallback.release()
	mustDo(t, ctxFallback.serveFile("x", openFS{open: func(string) (fs.File, error) {
		return &memoryFile{
			data: []byte("fallback-content"),
			info: fileInfoStub{name: "x.txt"},
		}, nil
	}}, ""))
	if body := recFallback.Body.String(); body != "fallback-content" {
		t.Fatalf("body=%q", body)
	}
}

type rendererErrorStub struct {
	err error
}

func (r rendererErrorStub) Render(io.Writer, string, any, *Context) error {
	return r.err
}

type openFS struct {
	open func(name string) (fs.File, error)
}

func (o openFS) Open(name string) (fs.File, error) {
	return o.open(name)
}

type statErrFile struct{}

func (f *statErrFile) Stat() (fs.FileInfo, error) {
	return nil, errors.New("stat failed")
}

func (f *statErrFile) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (f *statErrFile) Close() error {
	return nil
}

type readErrFile struct {
	info fs.FileInfo
	err  error
}

func (f *readErrFile) Stat() (fs.FileInfo, error) {
	return f.info, nil
}

func (f *readErrFile) Read([]byte) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	return 0, io.EOF
}

func (f *readErrFile) Close() error {
	return nil
}

type memoryFile struct {
	data []byte
	pos  int
	info fs.FileInfo
}

func (f *memoryFile) Stat() (fs.FileInfo, error) {
	return f.info, nil
}

func (f *memoryFile) Read(p []byte) (int, error) {
	if f.pos >= len(f.data) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.pos:])
	f.pos += n
	return n, nil
}

func (f *memoryFile) Close() error {
	return nil
}

type fileInfoStub struct {
	name string
	dir  bool
}

func (fi fileInfoStub) Name() string {
	return fi.name
}

func (fi fileInfoStub) Size() int64 {
	return 0
}

func (fi fileInfoStub) Mode() fs.FileMode {
	if fi.dir {
		return fs.ModeDir | 0o755
	}
	return 0
}

func (fi fileInfoStub) ModTime() time.Time {
	return time.Unix(0, 0)
}

func (fi fileInfoStub) IsDir() bool {
	return fi.dir
}

func (fi fileInfoStub) Sys() any {
	return nil
}

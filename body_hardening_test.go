package zinc_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"github.com/0mjs/zinc"
	"github.com/0mjs/zinc/middleware"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

func TestBindingBodyBudgetAcrossFormSources(t *testing.T) {
	for _, multipartBody := range []bool{false, true} {
		var body []byte
		contentType := "application/x-www-form-urlencoded"
		if multipartBody {
			var buffer bytes.Buffer
			writer := multipart.NewWriter(&buffer)
			part, err := writer.CreateFormFile("file", "file.txt")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(part, strings.Repeat("x", 128))
			_ = writer.Close()
			body, contentType = buffer.Bytes(), writer.FormDataContentType()
		} else {
			body = []byte("name=" + strings.Repeat("x", 128))
		}
		for _, source := range []string{"All", "Body", "Form", "MultipartForm", "FormFile"} {
			if !multipartBody && (source == "MultipartForm" || source == "FormFile") {
				continue
			}
			for _, knownLength := range []bool{false, true} {
				for _, exact := range []bool{false, true} {
					cfg := zinc.Config{}
					cfg.BodyLimit = int64(len(body))
					if !exact {
						cfg.BodyLimit--
					}
					app := zinc.New(cfg)
					app.Post("/", func(c *zinc.Context) error {
						var input struct {
							Name string `form:"name"`
						}
						bind := func() error {
							switch source {
							case "All":
								return c.Bind().All(&input)
							case "Body":
								return c.Bind().Body(&input)
							case "Form":
								return c.Bind().Form(&input)
							case "MultipartForm":
								_, err := c.MultipartForm()
								return err
							default:
								_, err := c.FormFile("file")
								return err
							}
						}
						err := bind()
						if !exact && err != nil && !errors.Is(err, zinc.ErrRequestEntityTooLarge) {
							t.Errorf("limit error not identifiable: %v", err)
						}
						if again := bind(); !exact && again == nil {
							t.Error("repeated binding hid the limit error")
						}
						if err != nil {
							return err
						}
						return c.String("ok")
					})
					r := httptest.NewRequest("POST", "/", bytes.NewReader(body))
					r.Header.Set("Content-Type", contentType)
					if !knownLength {
						r.ContentLength = -1
					}
					w := httptest.NewRecorder()
					app.ServeHTTP(w, r)
					want := 413
					if exact {
						want = 200
					}
					if w.Code != want {
						t.Fatalf("multipart=%v source=%s known=%v exact=%v: %d %q", multipartBody, source, knownLength, exact, w.Code, w.Body.String())
					}
				}
			}
		}
	}
}

func TestDefaultDecompressionBudgetAppliesToRawReaders(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = io.WriteString(writer, strings.Repeat("x", 1000))
	_ = writer.Close()
	cfg := zinc.Config{}
	cfg.BodyLimit = 64
	app := zinc.New(cfg)
	app.Use(middleware.Decompress())
	app.Post("/", func(c *zinc.Context) error { _, err := io.ReadAll(c.Request().Body); return err })
	r := httptest.NewRequest("POST", "/", bytes.NewReader(compressed.Bytes()))
	r.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatalf("unbounded expansion: %d", w.Code)
	}
}

func TestMultipartTemporaryFilesAreReleased(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "upload.txt")
	_, _ = io.WriteString(part, strings.Repeat("x", 4096))
	_ = writer.Close()
	var file *multipart.FileHeader
	app := zinc.New()
	app.Post("/", func(c *zinc.Context) error {
		if err := c.Request().ParseMultipartForm(1); err != nil {
			return err
		}
		var err error
		file, err = c.FormFile("file")
		return err
	})
	r := httptest.NewRequest("POST", "/", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	app.ServeHTTP(httptest.NewRecorder(), r)
	if file == nil {
		t.Fatal("missing file")
	}
	opened, err := file.Open()
	if err == nil {
		_ = opened.Close()
		t.Fatal("temporary upload retained after request release")
	}
}
func TestFormBindingHonorsBodyLimit(t *testing.T) {
	cfg := zinc.Config{}
	cfg.BodyLimit = 8
	app := zinc.New(cfg)
	app.Post("/", func(c *zinc.Context) error {
		var input struct {
			Name string `form:"name"`
		}
		if err := c.Bind().Form(&input); err != nil {
			return err
		}
		return c.String(input.Name)
	})
	r := httptest.NewRequest("POST", "/", strings.NewReader("name="+strings.Repeat("x", 32)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatalf("body limit 8 accepted 37 bytes: %d %q", w.Code, w.Body.String())
	}
}

func TestBodyLimitAcceptsExactBoundary(t *testing.T) {
	app := zinc.New()
	app.Use(middleware.BodyLimit(4))
	app.Post("/", func(c *zinc.Context) error {
		_, err := c.BodyBytes()
		if err != nil {
			return err
		}
		return c.String("ok")
	})
	w := httptest.NewRecorder()
	app.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader("1234")))
	if w.Code != 200 {
		t.Fatalf("exactly four bytes rejected: %d %q", w.Code, w.Body.String())
	}
}

func TestXMLPreallocationHonorsBodyLimit(t *testing.T) {
	cfg := zinc.Config{}
	cfg.BodyLimit = 64
	app := zinc.New(cfg)
	app.Post("/", func(c *zinc.Context) error {
		var input struct {
			Value string `xml:"value"`
		}
		return c.Bind().XML(&input)
	})
	r := httptest.NewRequest("POST", "/", strings.NewReader("<input><value>x</value></input>"))
	r.ContentLength = 16 << 20 // Bounded probe: header claims 16 MiB, actual body is tiny.
	w := httptest.NewRecorder()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	app.ServeHTTP(w, r)
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	if allocated > 1<<20 {
		t.Fatalf("64-byte body limit still allocated %d bytes for claimed Content-Length", allocated)
	}
}

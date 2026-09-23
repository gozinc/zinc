// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package zinc

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type validatingStub struct{}

func (validatingStub) Validate(v any) error {
	if payload, ok := v.(*bindPayload); ok && payload.Name == "" {
		return ErrBadRequest.WithMessage("name required")
	}
	return nil
}

type bindPayload struct {
	ID    int      `path:"id"`
	Page  int      `query:"page"`
	Name  string   `json:"name" form:"name"`
	Auth  string   `header:"x-auth"`
	Tags  []string `form:"tags"`
	Ready bool     `query:"ready"`
}

type textInt int

func (i *textInt) UnmarshalText(text []byte) error {
	parsed, err := strconv.Atoi(string(text))
	if err != nil {
		return err
	}
	*i = textInt(parsed)
	return nil
}

func BenchmarkContextAcquireRelease(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewContext(rec, req)
		ctx.release()
	}
}

func TestContextReleaseResetDoesNotLeakRequestState(t *testing.T) {
	firstRequest := httptest.NewRequest(http.MethodPost, "/first?dirty=true", strings.NewReader("body"))
	secondRequest := httptest.NewRequest(http.MethodGet, "/second", nil)
	firstWriter := httptest.NewRecorder()
	secondWriter := httptest.NewRecorder()
	ctx := NewContext(firstWriter, firstRequest)

	ctx.queryParams = url.Values{"dirty": {"true"}}
	ctx.handlers = []HandlerFunc{func(*Context) error { return nil }}
	ctx.body = []byte("body")
	ctx.bodyRead = true
	ctx.bodyErr = errors.New("dirty body")
	ctx.sameSite = http.SameSiteStrictMode
	ctx.paramPath = "/first"
	ctx.paramRoute = &radixRoute{}
	ctx.written = true
	ctx.index = 4
	ctx.status = http.StatusCreated
	ctx.app = New()
	ctx.routeInfo = routeMeta{path: "/first"}
	ctx.routeIndex = 3
	ctx.routeIndexed = true
	ctx.lastErr = errors.New("dirty handler")
	ctx.setParam("id", "42")
	ctx.Set("dirty", true)

	ctx.release()
	reused := NewContext(secondWriter, secondRequest)
	defer reused.release()

	if reused.writer != secondWriter || reused.request != secondRequest {
		t.Fatal("writer or request leaked across context reuse")
	}
	if reused.queryParams != nil || reused.handlers != nil || reused.body != nil || reused.bodyRead || reused.bodyErr != nil {
		t.Fatal("request data leaked across context reuse")
	}
	if reused.sameSite != 0 || reused.paramPath != "" || reused.paramRoute != nil || reused.paramCount != 0 {
		t.Fatal("routing data leaked across context reuse")
	}
	if reused.written || reused.index != -1 || reused.status != http.StatusOK || reused.app != nil {
		t.Fatal("response state leaked across context reuse")
	}
	if reused.routeInfo.path != "" || reused.routeInfo.method != "" || len(reused.routeInfo.params) != 0 ||
		reused.routeIndex != -1 || reused.routeIndexed || reused.lastErr != nil {
		t.Fatal("route metadata leaked across context reuse")
	}
	if len(reused.store) != 0 {
		t.Fatal("context store leaked across context reuse")
	}
}

func TestContextRequestHelpersAndMetadata(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/users/42?page=3&ready=true", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set(HeaderXForwardedFor, "203.0.113.8, 10.0.0.1")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set(HeaderXRequestID, "req-123")
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})

	ctx, resp := newRecorderContext(t, req)
	defer ctx.release()
	ctx.app = NewWithConfig(Config{ProxyHeader: HeaderXForwardedFor, TrustedProxies: []string{"10.0.0.1"}})
	ctx.setParam("id", "42")

	ctx.Set("key", "value")
	if got, ok := ctx.Get("key"); !ok || got != "value" {
		t.Fatalf("Get = %v %v", got, ok)
	}
	if ctx.MustGet("key") != "value" {
		t.Fatal("MustGet returned wrong value")
	}

	if ctx.Method() != http.MethodGet {
		t.Fatalf("method=%q", ctx.Method())
	}
	if ctx.Path() != "/users/42" {
		t.Fatalf("path=%q", ctx.Path())
	}
	if ctx.OriginalURL() != "/users/42?page=3&ready=true" {
		t.Fatalf("original=%q", ctx.OriginalURL())
	}
	ctx.SetPath("/users/99")
	if ctx.Path() != "/users/99" {
		t.Fatalf("path=%q", ctx.Path())
	}
	if ctx.Param("id") != "42" || ctx.ParamOr("missing", "fallback") != "fallback" {
		t.Fatal("param helpers failed")
	}
	if ctx.Query("page") != "3" || ctx.QueryOr("missing", "fallback") != "fallback" {
		t.Fatal("query helpers failed")
	}
	if ctx.QueryValues().Encode() != (url.Values{"page": []string{"3"}, "ready": []string{"true"}}).Encode() {
		t.Fatalf("query values=%v", ctx.QueryValues())
	}
	if ctx.GetHeader(HeaderXRequestID) != "req-123" {
		t.Fatal("header lookup failed")
	}
	cookie, err := ctx.Cookie("session")
	mustDo(t, err)
	if cookie.Value != "abc" || len(ctx.Cookies()) != 1 {
		t.Fatal("cookie helpers failed")
	}
	if ctx.Scheme() != "https" || !ctx.Secure() {
		t.Fatal("scheme helpers failed")
	}
	if ctx.IP() != "203.0.113.8" {
		t.Fatalf("ip=%q", ctx.IP())
	}
	if len(ctx.IPs()) != 2 || ctx.RemoteIP() != "10.0.0.1" {
		t.Fatalf("ips=%v remote=%q", ctx.IPs(), ctx.RemoteIP())
	}
	preflightReq := httptest.NewRequest(http.MethodOptions, "http://example.com", nil)
	preflightReq.Header.Set(HeaderAccessControlRequestMethod, http.MethodPost)
	preflightCtx, _ := newRecorderContext(t, preflightReq)
	defer preflightCtx.release()
	if !preflightCtx.IsPreflight() {
		t.Fatal("expected preflight request")
	}
	if ctx.RequestID() != "req-123" {
		t.Fatal("request id helper failed")
	}

	valueCtx := stdctx.WithValue(ctx.Context(), "key", "value")
	ctx.SetContext(valueCtx)
	if got := ctx.Context().Value("key"); got != "value" {
		t.Fatalf("context value=%v", got)
	}
	ctx.SetWriter(resp)
	ctx.SetRequest(req)
	if ctx.Writer() != resp || ctx.Request() != req {
		t.Fatal("raw writer/request helpers failed")
	}
}

func TestContextTypedStoreGetters(t *testing.T) {
	ctx, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctx.release()

	ctx.Set("string", "zinc")
	ctx.Set("bool", true)
	ctx.Set("int", 7)
	ctx.Set("int64", int64(9))
	ctx.Set("float64", 1.5)
	ctx.Set("strings", []string{"a", "b"})
	ctx.Set("map", Map{"ok": true})
	ctx.Set("mapString", map[string]string{"name": "zinc"})
	ctx.Set("mapStringSlice", map[string][]string{"tags": {"api", "go"}})

	if ctx.GetString("string") != "zinc" ||
		!ctx.GetBool("bool") ||
		ctx.GetInt("int") != 7 ||
		ctx.GetInt64("int64") != 9 ||
		ctx.GetFloat64("float64") != 1.5 {
		t.Fatal("scalar typed getters failed")
	}
	if got := ctx.GetStringSlice("strings"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("string slice=%v", got)
	}
	if got := ctx.GetStringMap("map"); got["ok"] != true {
		t.Fatalf("string map=%v", got)
	}
	if got := ctx.GetStringMapString("mapString"); got["name"] != "zinc" {
		t.Fatalf("string map string=%v", got)
	}
	if got := ctx.GetStringMapStringSlice("mapStringSlice"); len(got["tags"]) != 2 {
		t.Fatalf("string map string slice=%v", got)
	}

	if ctx.GetString("missing") != "" ||
		ctx.GetBool("missing") ||
		ctx.GetInt("missing") != 0 ||
		ctx.GetInt64("missing") != 0 ||
		ctx.GetFloat64("missing") != 0 {
		t.Fatal("missing scalar getters should return zero values")
	}
	if ctx.GetString("int") != "" || ctx.GetInt("string") != 0 {
		t.Fatal("mismatched typed getters should return zero values")
	}
	if ctx.GetStringSlice("missing") != nil ||
		ctx.GetStringMap("missing") != nil ||
		ctx.GetStringMapString("missing") != nil ||
		ctx.GetStringMapStringSlice("missing") != nil {
		t.Fatal("missing collection getters should return nil")
	}
}

func TestContextQueryAndPostFormCollections(t *testing.T) {
	t.Run("query arrays and maps", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/search?tag=a&tag=b&filter[name]=zinc&filter[role]=admin", nil)
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		if got := ctx.QueryArray("tag"); !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Fatalf("query array=%v", got)
		}
		if got := ctx.QueryArray("missing"); len(got) != 0 {
			t.Fatalf("missing query array=%v", got)
		}
		queryMap := ctx.QueryMap("filter")
		if queryMap["name"] != "zinc" || queryMap["role"] != "admin" {
			t.Fatalf("query map=%v", queryMap)
		}
		if got := ctx.QueryMap("missing"); len(got) != 0 {
			t.Fatalf("missing query map=%v", got)
		}
	})

	t.Run("urlencoded post form arrays and maps", func(t *testing.T) {
		form := url.Values{
			"name":       {"body"},
			"tag":        {"a", "b"},
			"user[name]": {"zinc"},
			"user[role]": {"admin"},
		}
		req := httptest.NewRequest(http.MethodPost, "/submit?name=query&queryOnly=yes", strings.NewReader(form.Encode()))
		req.Header.Set(HeaderContentType, "application/x-www-form-urlencoded")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		if got := ctx.PostForm("name"); got != "body" {
			t.Fatalf("post form name=%q", got)
		}
		if got := ctx.PostForm("queryOnly"); got != "" {
			t.Fatalf("post form should not read query values, got=%q", got)
		}
		if got := ctx.PostFormOr("missing", "fallback"); got != "fallback" {
			t.Fatalf("post form fallback=%q", got)
		}
		if got := ctx.PostFormArray("tag"); !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Fatalf("post form array=%v", got)
		}
		postFormMap := ctx.PostFormMap("user")
		if postFormMap["name"] != "zinc" || postFormMap["role"] != "admin" {
			t.Fatalf("post form map=%v", postFormMap)
		}
	})

	t.Run("multipart post form arrays and maps", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		mustDo(t, writer.WriteField("tag", "a"))
		mustDo(t, writer.WriteField("tag", "b"))
		mustDo(t, writer.WriteField("user[name]", "zinc"))
		mustDo(t, writer.WriteField("user[role]", "admin"))
		mustDo(t, writer.Close())

		req := httptest.NewRequest(http.MethodPost, "/submit", &body)
		req.Header.Set(HeaderContentType, writer.FormDataContentType())
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		if got := ctx.PostFormArray("tag"); !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Fatalf("multipart post form array=%v", got)
		}
		postFormMap := ctx.PostFormMap("user")
		if postFormMap["name"] != "zinc" || postFormMap["role"] != "admin" {
			t.Fatalf("multipart post form map=%v", postFormMap)
		}
	})
}

func TestContextRequestIntrospectionHelpers(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderContentType, "Application/JSON; charset=utf-8")
	req.Header.Set(HeaderConnection, "keep-alive, Upgrade")
	req.Header.Set(HeaderUpgrade, "WebSocket")
	ctx, _ := newRecorderContext(t, req)
	defer ctx.release()

	if got := ctx.ContentType(); got != "application/json" {
		t.Fatalf("content type=%q", got)
	}
	if !ctx.IsWebSocket() {
		t.Fatal("expected websocket request")
	}

	plainReq := httptest.NewRequest(http.MethodGet, "/", nil)
	plainReq.Header.Set(HeaderConnection, "close")
	plainCtx, _ := newRecorderContext(t, plainReq)
	defer plainCtx.release()
	if plainCtx.ContentType() != "" || plainCtx.IsWebSocket() {
		t.Fatal("plain request should not report content type or websocket")
	}
}

func TestContextAcquireRelease(t *testing.T) {
	app := New()

	acquiredReq := httptest.NewRequest(http.MethodGet, "/acquire?x=1", nil)
	acquired := app.AcquireContext(httptest.NewRecorder(), acquiredReq)
	if acquired.app != app {
		t.Fatal("AcquireContext should attach app")
	}
	if acquired.Path() != "/acquire" {
		t.Fatalf("path=%q", acquired.Path())
	}
	app.ReleaseContext(acquired)
	app.ReleaseContext(nil)
}

func TestContextFailReturnsError(t *testing.T) {
	ctx, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer ctx.release()

	err := errors.New("failed")
	if got := ctx.Fail(err); !errors.Is(got, err) {
		t.Fatalf("fail error=%v", got)
	}
}

func TestContextBodyBindingAndUploads(t *testing.T) {
	t.Run("body bytes and bind", func(t *testing.T) {
		app := NewWithConfig(Config{Validator: validatingStub{}})
		body := strings.NewReader(`{"name":"matt"}`)
		req := httptest.NewRequest(http.MethodPost, "/users/7?page=2&ready=true", body)
		req.Header.Set(HeaderContentType, "application/json")
		req.Header.Set("X-Auth", "secret")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()
		ctx.app = app
		ctx.setParam("id", "7")

		bytesBody, err := ctx.BodyBytes()
		mustDo(t, err)
		if string(bytesBody) != `{"name":"matt"}` {
			t.Fatalf("body=%q", string(bytesBody))
		}
		stringBody, err := ctx.BodyString()
		mustDo(t, err)
		if stringBody != `{"name":"matt"}` {
			t.Fatalf("body=%q", stringBody)
		}

		var payload bindPayload
		mustDo(t, ctx.Bind().All(&payload))
		mustDo(t, ctx.Bind().Header(&payload))
		if payload.ID != 7 || payload.Page != 2 || payload.Name != "matt" || payload.Auth != "secret" || !payload.Ready {
			t.Fatalf("payload=%+v", payload)
		}

		var builderPayload bindPayload
		mustDo(t, ctx.Bind().JSON(&builderPayload))
		if builderPayload.Name != "matt" {
			t.Fatalf("builder payload=%+v", builderPayload)
		}
		bodyAgain, err := ctx.BodyBytes()
		mustDo(t, err)
		if string(bodyAgain) != `{"name":"matt"}` {
			t.Fatalf("body after bind=%q", string(bodyAgain))
		}
	})

	t.Run("xml and form binding", func(t *testing.T) {
		app := NewWithConfig(Config{Validator: validatingStub{}})

		xmlReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`<bindPayload><Name>zoe</Name></bindPayload>`))
		xmlReq.Header.Set(HeaderContentType, "application/xml")
		xmlCtx, _ := newRecorderContext(t, xmlReq)
		defer xmlCtx.release()
		xmlCtx.app = app
		var xmlPayload struct {
			Name string `xml:"Name"`
		}
		mustDo(t, xmlCtx.Bind().XML(&xmlPayload))
		if xmlPayload.Name != "zoe" {
			t.Fatalf("xml payload=%+v", xmlPayload)
		}

		form := url.Values{"name": {"mia"}, "tags": {"a", "b"}}
		formReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
		formReq.Header.Set(HeaderContentType, "application/x-www-form-urlencoded")
		formCtx, _ := newRecorderContext(t, formReq)
		defer formCtx.release()
		formCtx.app = app
		var formPayload bindPayload
		mustDo(t, formCtx.Bind().Form(&formPayload))
		if formPayload.Name != "mia" || len(formPayload.Tags) != 2 {
			t.Fatalf("form payload=%+v", formPayload)
		}

		var builderFormPayload bindPayload
		mustDo(t, formCtx.Bind().Form(&builderFormPayload))
		if builderFormPayload.Name != "mia" || len(builderFormPayload.Tags) != 2 {
			t.Fatalf("builder form payload=%+v", builderFormPayload)
		}
	})

	t.Run("validator failure", func(t *testing.T) {
		app := NewWithConfig(Config{Validator: validatingStub{}})
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":""}`))
		req.Header.Set(HeaderContentType, "application/json")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()
		ctx.app = app
		var payload bindPayload
		if err := ctx.Bind().All(&payload); err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("multipart upload helpers", func(t *testing.T) {
		app := New()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "hello.txt")
		mustDo(t, err)
		_, err = part.Write([]byte("upload"))
		mustDo(t, err)
		part2, err := writer.CreateFormFile("file", "hello-2.txt")
		mustDo(t, err)
		_, err = part2.Write([]byte("upload-2"))
		mustDo(t, err)
		mustDo(t, writer.WriteField("name", "doc"))
		mustDo(t, writer.Close())

		req := httptest.NewRequest(http.MethodPost, "/", &body)
		req.Header.Set(HeaderContentType, writer.FormDataContentType())
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()
		ctx.app = app

		file, err := ctx.FormFile("file")
		mustDo(t, err)
		files, err := ctx.FormFiles("file")
		mustDo(t, err)
		form, err := ctx.MultipartForm()
		mustDo(t, err)
		if form.Value["name"][0] != "doc" || len(files) != 2 || file.Filename != "hello.txt" {
			t.Fatal("multipart helpers failed")
		}

		dst := filepath.Join(t.TempDir(), "saved.txt")
		mustDo(t, ctx.SaveFile(file, dst))
		data, err := os.ReadFile(dst)
		mustDo(t, err)
		if string(data) != "upload" {
			t.Fatalf("saved file=%q", string(data))
		}

		var uploadPayload struct {
			Name      string                  `form:"name"`
			File      *multipart.FileHeader   `form:"file"`
			Files     []*multipart.FileHeader `form:"file"`
			FileValue multipart.FileHeader    `form:"file"`
			FileList  []multipart.FileHeader  `form:"file"`
		}
		mustDo(t, ctx.Bind().Form(&uploadPayload))
		if uploadPayload.Name != "doc" ||
			uploadPayload.File == nil ||
			uploadPayload.File.Filename != "hello.txt" ||
			len(uploadPayload.Files) != 2 ||
			uploadPayload.FileValue.Filename != "hello.txt" ||
			len(uploadPayload.FileList) != 2 {
			t.Fatalf("upload payload=%+v", uploadPayload)
		}

		var builderUploadPayload struct {
			Name string                `form:"name"`
			File *multipart.FileHeader `form:"file"`
		}
		mustDo(t, ctx.Bind().Form(&builderUploadPayload))
		if builderUploadPayload.Name != "doc" || builderUploadPayload.File == nil || builderUploadPayload.File.Filename != "hello.txt" {
			t.Fatalf("builder upload payload=%+v", builderUploadPayload)
		}
	})
}

func TestContextRouteAndJSONEncodingHelpers(t *testing.T) {
	app := New()
	app.Get("/meta/{id}", func(c *Context) error {
		return c.JSON(Map{"path": c.FullPath(), "route": c.Route(), "id": c.Param("id")})
	})
	resp := performRequest(t, app, http.MethodGet, "/meta/11", nil, nil)
	var payload map[string]any
	mustDo(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	if payload["path"] != "/meta/{id}" || payload["id"] != "11" {
		t.Fatalf("payload=%v", payload)
	}
}

func TestBindDataPopulatesSupportedFieldKinds(t *testing.T) {
	type payload struct {
		Name       string   `query:"name"`
		Enabled    bool     `query:"enabled"`
		Count      int      `query:"count"`
		Total      uint     `query:"total"`
		Ratio      float64  `query:"ratio"`
		Labels     []string `query:"labels"`
		IDs        []int    `query:"ids"`
		Optional   string   `query:"optional,omitempty"`
		DefaultVal string
		Ignored    string `query:"-"`
	}

	input := map[string][]string{
		"name":       {"zinc"},
		"enabled":    {"true"},
		"count":      {"7"},
		"total":      {"12"},
		"ratio":      {"2.5"},
		"labels":     {"a", "b"},
		"ids":        {"1", "2"},
		"optional":   {"opt"},
		"defaultval": {"default"},
		"ignored":    {"should-not-apply"},
	}

	var got payload
	if err := bindData(&got, input, "query"); err != nil {
		t.Fatalf("bindData err=%v", err)
	}

	if got.Name != "zinc" || !got.Enabled || got.Count != 7 || got.Total != 12 {
		t.Fatalf("unexpected scalar values: %+v", got)
	}
	if got.Ratio != 2.5 {
		t.Fatalf("ratio=%v", got.Ratio)
	}
	if len(got.Labels) != 2 || got.Labels[0] != "a" || got.Labels[1] != "b" {
		t.Fatalf("labels=%v", got.Labels)
	}
	if len(got.IDs) != 2 || got.IDs[0] != 1 || got.IDs[1] != 2 {
		t.Fatalf("ids=%v", got.IDs)
	}
	if got.Optional != "opt" {
		t.Fatalf("optional=%q", got.Optional)
	}
	if got.DefaultVal != "default" {
		t.Fatalf("default value=%q", got.DefaultVal)
	}
	if got.Ignored != "" {
		t.Fatalf("ignored should not be set, got=%q", got.Ignored)
	}
}

func TestBindDataHeaderLookupIsCaseInsensitive(t *testing.T) {
	var got struct {
		Token string `header:"X-Token"`
	}
	if err := bindData(&got, map[string][]string{"x-token": {"abc"}}, "header"); err != nil {
		t.Fatalf("bindData err=%v", err)
	}
	if got.Token != "abc" {
		t.Fatalf("token=%q", got.Token)
	}
}

func TestBindDataReturnsValidationErrorsForBadTargets(t *testing.T) {
	err := bindData(nil, map[string][]string{}, "query")
	if err == nil || !strings.Contains(err.Error(), "must not be nil") {
		t.Fatalf("err=%v", err)
	}

	err = bindData(struct{}{}, map[string][]string{}, "query")
	if err == nil || !strings.Contains(err.Error(), "must be a pointer") {
		t.Fatalf("err=%v", err)
	}

	value := 1
	err = bindData(&value, map[string][]string{}, "query")
	if err == nil || !strings.Contains(err.Error(), "must point to a struct") {
		t.Fatalf("err=%v", err)
	}
}

func TestBindDataWrapsFieldConversionErrors(t *testing.T) {
	var got struct {
		Count int `query:"count"`
	}
	err := bindData(&got, map[string][]string{"count": {"not-a-number"}}, "query")
	if err == nil || !strings.Contains(err.Error(), "bind Count") {
		t.Fatalf("err=%v", err)
	}

	var unsupported struct {
		Flags []bool `query:"flags"`
	}
	err = bindData(&unsupported, map[string][]string{"flags": {"true"}}, "query")
	if err == nil || !strings.Contains(err.Error(), "unsupported slice element type") {
		t.Fatalf("err=%v", err)
	}
}

func TestSetFieldValueCoversEdgeCases(t *testing.T) {
	// Unsettable values should be ignored.
	if err := setFieldValue(reflect.ValueOf(10), []string{"12"}); err != nil {
		t.Fatalf("err=%v", err)
	}

	target := 10
	if err := setFieldValue(reflect.ValueOf(&target).Elem(), nil); err != nil {
		t.Fatalf("err=%v", err)
	}
	if target != 10 {
		t.Fatalf("target=%d", target)
	}

	var bad struct {
		Field struct{}
	}
	err := setFieldValue(reflect.ValueOf(&bad).Elem().FieldByName("Field"), []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("err=%v", err)
	}

	var vBool struct{ V bool }
	err = setFieldValue(reflect.ValueOf(&vBool).Elem().Field(0), []string{"not-bool"})
	if err == nil {
		t.Fatal("expected parse bool error")
	}

	var vInt struct{ V int }
	err = setFieldValue(reflect.ValueOf(&vInt).Elem().Field(0), []string{"x"})
	if err == nil {
		t.Fatal("expected parse int error")
	}

	var vUint struct{ V uint }
	err = setFieldValue(reflect.ValueOf(&vUint).Elem().Field(0), []string{"x"})
	if err == nil {
		t.Fatal("expected parse uint error")
	}

	var vFloat struct{ V float64 }
	err = setFieldValue(reflect.ValueOf(&vFloat).Elem().Field(0), []string{"x"})
	if err == nil {
		t.Fatal("expected parse float error")
	}
}

func TestDefaultBinderBindAndBindBodyBranches(t *testing.T) {
	binder := defaultBinder{codec: defaultJSONCodec{}}

	t.Run("Bind handles nil body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/users/9?page=3", nil)
		req.Body = nil
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got struct {
			ID   int `path:"id"`
			Page int `query:"page"`
		}
		ctx.setParam("id", "9")
		mustDo(t, binder.Bind(ctx, &got))
		if got.ID != 9 || got.Page != 3 {
			t.Fatalf("payload=%+v", got)
		}
	})

	t.Run("Bind handles http.NoBody", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/users/9?page=3", nil)
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got struct {
			ID   int `path:"id"`
			Page int `query:"page"`
		}
		ctx.setParam("id", "9")
		mustDo(t, binder.Bind(ctx, &got))
		if got.ID != 9 || got.Page != 3 {
			t.Fatalf("payload=%+v", got)
		}
	})

	t.Run("Bind empty body validates only", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/users/9", strings.NewReader(""))
		req.Header.Set(HeaderContentType, "application/json")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got struct{}
		mustDo(t, binder.Bind(ctx, &got))
	})

	t.Run("Bind unsupported content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("x"))
		req.Header.Set(HeaderContentType, "application/octet-stream")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		err := binder.Bind(ctx, &struct{}{})
		if err == nil || !strings.Contains(err.Error(), "unsupported content type") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("Bind text plain into string", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
		req.Header.Set(HeaderContentType, "text/plain; charset=utf-8")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got string
		mustDo(t, binder.Bind(ctx, &got))
		if got != "hello" {
			t.Fatalf("got=%q", got)
		}
	})

	t.Run("Bind YAML into struct", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("name: lin\n"))
		req.Header.Set(HeaderContentType, "application/x-yaml")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got struct {
			Name string `yaml:"name"`
		}
		mustDo(t, binder.Bind(ctx, &got))
		if got.Name != "lin" {
			t.Fatalf("got=%+v", got)
		}
	})

	t.Run("BindBody empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		req.Header.Set(HeaderContentType, "application/json")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		err := binder.BindBody(ctx, &struct{}{})
		if err == nil || !strings.Contains(err.Error(), "request body is empty") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("BindBody text plain into bytes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
		req.Header.Set(HeaderContentType, "text/plain")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got []byte
		mustDo(t, binder.BindBody(ctx, &got))
		if string(got) != "hello" {
			t.Fatalf("got=%q", string(got))
		}
	})

	t.Run("BindBody TOML into struct", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("name = 'lin'\n"))
		req.Header.Set(HeaderContentType, "application/toml")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got struct {
			Name string `toml:"name"`
		}
		mustDo(t, binder.BindBody(ctx, &got))
		if got.Name != "lin" {
			t.Fatalf("got=%+v", got)
		}
	})

	t.Run("BindBody text plain empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		req.Header.Set(HeaderContentType, "text/plain")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		err := binder.BindBody(ctx, new(string))
		if err == nil || !strings.Contains(err.Error(), "request body is empty") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("BindBody XML decode error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("<broken"))
		req.Header.Set(HeaderContentType, "application/xml")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		err := binder.BindBody(ctx, &struct{}{})
		if err == nil || !strings.Contains(err.Error(), "bind body") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("BindBody form delegates to BindForm", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("name=lin"))
		req.Header.Set(HeaderContentType, "application/x-www-form-urlencoded")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got struct {
			Name string `form:"name"`
		}
		mustDo(t, binder.BindBody(ctx, &got))
		if got.Name != "lin" {
			t.Fatalf("name=%q", got.Name)
		}
	})

	t.Run("BindBody unsupported content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("x"))
		req.Header.Set(HeaderContentType, "application/octet-stream")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		err := binder.BindBody(ctx, &struct{}{})
		if err == nil || !strings.Contains(err.Error(), "unsupported content type") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("BindForm parse form failure", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set(HeaderContentType, "application/x-www-form-urlencoded")
		req.Body = errReadCloser{err: errors.New("read failed")}
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		err := binder.BindForm(ctx, &struct{}{})
		if err == nil || !strings.Contains(err.Error(), "parse form") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestBindXMLAndValidateBranches(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.Header.Set(HeaderContentType, "application/xml")
	ctx, _ := newRecorderContext(t, req)
	defer ctx.release()

	var payload struct{}
	err := ctx.Bind().XML(&payload)
	if err == nil || !strings.Contains(err.Error(), "request body is empty") {
		t.Fatalf("err=%v", err)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("<broken"))
	req2.Header.Set(HeaderContentType, "application/xml")
	ctx2, _ := newRecorderContext(t, req2)
	defer ctx2.release()

	err = ctx2.Bind().XML(&payload)
	if err == nil {
		t.Fatal("expected XML decode error")
	}

	// Validate should be a no-op without app/validator.
	var nilAppCtx Context
	if err := nilAppCtx.Validate(payload); err != nil {
		t.Fatalf("err=%v", err)
	}

	noValidatorCtx, _ := newRecorderContext(t, httptest.NewRequest(http.MethodGet, "/", nil))
	defer noValidatorCtx.release()
	noValidatorCtx.app = New()
	if err := noValidatorCtx.Validate(payload); err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestBindTextAndBindingText(t *testing.T) {
	t.Run("Context BindText supports TextUnmarshaler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("42"))
		req.Header.Set(HeaderContentType, "text/plain")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got textInt
		mustDo(t, ctx.Bind().Text(&got))
		if got != 42 {
			t.Fatalf("got=%d", got)
		}
	})

	t.Run("Binding Text rejects unsupported target", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
		req.Header.Set(HeaderContentType, "text/plain")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		err := ctx.Bind().Text(&struct{}{})
		if err == nil || !strings.Contains(err.Error(), "unsupported kind struct") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestBindYAMLAndBindTOML(t *testing.T) {
	t.Run("Context BindYAML", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("name: lin\n"))
		req.Header.Set(HeaderContentType, "application/yaml")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got struct {
			Name string `yaml:"name"`
		}
		mustDo(t, ctx.Bind().YAML(&got))
		if got.Name != "lin" {
			t.Fatalf("got=%+v", got)
		}
	})

	t.Run("Binding TOML", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("count = 7\n"))
		req.Header.Set(HeaderContentType, "application/toml")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got struct {
			Count int `toml:"count"`
		}
		mustDo(t, ctx.Bind().TOML(&got))
		if got.Count != 7 {
			t.Fatalf("got=%+v", got)
		}
	})

	t.Run("BindYAML empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		req.Header.Set(HeaderContentType, "application/yaml")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		err := ctx.Bind().YAML(&struct{}{})
		if err == nil || !strings.Contains(err.Error(), "request body is empty") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("BindTOML decode error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("count = ["))
		req.Header.Set(HeaderContentType, "application/toml")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		var got struct {
			Count int `toml:"count"`
		}
		err := ctx.Bind().TOML(&got)
		if err == nil {
			t.Fatal("expected TOML decode error")
		}
	})
}

func TestBinderAdditionalErrorBranches(t *testing.T) {
	binder := defaultBinder{codec: defaultJSONCodec{}}

	t.Run("Bind path and query binding errors", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/?count=bad", strings.NewReader(`{"name":"ok"}`))
		req.Header.Set(HeaderContentType, "application/json")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()
		ctx.setParam("id", "bad")

		var payload struct {
			ID    int `path:"id"`
			Count int `query:"count"`
			Name  string
		}
		err := binder.Bind(ctx, &payload)
		if err == nil {
			t.Fatal("expected bind error from invalid path/query data")
		}
		var bindErr *BindError
		if !errors.As(err, &bindErr) {
			t.Fatalf("err=%T", err)
		}
		if bindErr.Source != "path" || bindErr.Field != "ID" {
			t.Fatalf("bind err=%+v", bindErr)
		}
	})

	t.Run("Bind body reader error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set(HeaderContentType, "application/json")
		req.Body = errReadCloser{err: errors.New("read failed")}
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		err := binder.Bind(ctx, &struct{}{})
		if err == nil || !strings.Contains(err.Error(), "read failed") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("Bind JSON, XML and form decode errors", func(t *testing.T) {
		jsonReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
		jsonReq.Header.Set(HeaderContentType, "application/json")
		jsonCtx, _ := newRecorderContext(t, jsonReq)
		defer jsonCtx.release()
		if err := binder.Bind(jsonCtx, &struct{}{}); err == nil || !strings.Contains(err.Error(), "bind body") {
			t.Fatalf("err=%v", err)
		}

		xmlReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("<broken"))
		xmlReq.Header.Set(HeaderContentType, "application/xml")
		xmlCtx, _ := newRecorderContext(t, xmlReq)
		defer xmlCtx.release()
		if err := binder.Bind(xmlCtx, &struct{}{}); err == nil || !strings.Contains(err.Error(), "bind body") {
			t.Fatalf("err=%v", err)
		}

		formReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("count=bad"))
		formReq.Header.Set(HeaderContentType, "application/x-www-form-urlencoded")
		formCtx, _ := newRecorderContext(t, formReq)
		defer formCtx.release()
		var formPayload struct {
			Count int `form:"count"`
		}
		if err := binder.Bind(formCtx, &formPayload); err == nil {
			t.Fatal("expected bindData error for invalid form int")
		}
	})

	t.Run("BindBody body read and JSON decode errors", func(t *testing.T) {
		readErrReq := httptest.NewRequest(http.MethodPost, "/", nil)
		readErrReq.Header.Set(HeaderContentType, "application/json")
		readErrReq.Body = errReadCloser{err: errors.New("read failed")}
		readErrCtx, _ := newRecorderContext(t, readErrReq)
		defer readErrCtx.release()
		if err := binder.BindBody(readErrCtx, &struct{}{}); err == nil || !strings.Contains(err.Error(), "read failed") {
			t.Fatalf("err=%v", err)
		}

		decodeErrReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
		decodeErrReq.Header.Set(HeaderContentType, "application/json")
		decodeErrCtx, _ := newRecorderContext(t, decodeErrReq)
		defer decodeErrCtx.release()
		if err := binder.BindBody(decodeErrCtx, &struct{}{}); err == nil || !strings.Contains(err.Error(), "bind body") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("BindQuery BindForm BindHeader BindPath errors", func(t *testing.T) {
		queryReq := httptest.NewRequest(http.MethodGet, "/?count=bad", nil)
		queryCtx, _ := newRecorderContext(t, queryReq)
		defer queryCtx.release()
		var queryPayload struct {
			Count int `query:"count"`
		}
		if err := binder.BindQuery(queryCtx, &queryPayload); err == nil {
			t.Fatal("expected query bind error")
		}

		formReq := httptest.NewRequest(http.MethodPost, "/", nil)
		formReq.Header.Set(HeaderContentType, "application/x-www-form-urlencoded")
		formReq.Body = errReadCloser{err: errors.New("read failed")}
		formCtx, _ := newRecorderContext(t, formReq)
		defer formCtx.release()
		if err := binder.BindForm(formCtx, &struct{}{}); err == nil {
			t.Fatal("expected form parse error")
		}

		headerReq := httptest.NewRequest(http.MethodGet, "/", nil)
		headerReq.Header.Set("X-Count", "bad")
		headerCtx, _ := newRecorderContext(t, headerReq)
		defer headerCtx.release()
		var headerPayload struct {
			Count int `header:"x-count"`
		}
		if err := binder.BindHeader(headerCtx, &headerPayload); err == nil {
			t.Fatal("expected header bind error")
		}

		pathReq := httptest.NewRequest(http.MethodGet, "/", nil)
		pathCtx, _ := newRecorderContext(t, pathReq)
		defer pathCtx.release()
		pathCtx.setParam("id", "bad")
		var pathPayload struct {
			ID int `path:"id"`
		}
		if err := binder.BindPath(pathCtx, &pathPayload); err == nil {
			t.Fatal("expected path bind error")
		}
	})

	t.Run("BindXML body read error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = errReadCloser{err: errors.New("read failed")}
		req.Header.Set(HeaderContentType, "application/xml")
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()
		var payload struct{}
		if err := ctx.Bind().XML(&payload); err == nil || !strings.Contains(err.Error(), "read failed") {
			t.Fatalf("err=%v", err)
		}
	})
}

type errReadCloser struct {
	err error
}

func (r errReadCloser) Read([]byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	return 0, io.EOF
}

func (r errReadCloser) Close() error {
	return nil
}

func TestContextNilRequestFallbacks(t *testing.T) {
	var c Context

	if c.Context() == nil {
		t.Fatal("context should default to background")
	}
	if c.Context() != stdctx.Background() {
		t.Fatal("expected background context when request is nil")
	}
	c.SetContext(stdctx.WithValue(stdctx.Background(), "k", "v"))

	if c.Method() != "" || c.Path() != "" || c.OriginalURL() != "" {
		t.Fatalf("unexpected request metadata: method=%q path=%q url=%q", c.Method(), c.Path(), c.OriginalURL())
	}
	c.SetPath("/ignored")

	if values := c.QueryValues(); len(values) != 0 {
		t.Fatalf("query values=%v", values)
	}
	if c.FormValue("name") != "" {
		t.Fatalf("form value=%q", c.FormValue("name"))
	}

	if _, err := c.FormFile("file"); err == nil {
		t.Fatal("expected FormFile error with nil request")
	}
	if _, err := c.FormFiles("file"); err == nil {
		t.Fatal("expected FormFiles error with nil request")
	}
	if _, err := c.MultipartForm(); err == nil {
		t.Fatal("expected MultipartForm error with nil request")
	}

	if c.GetHeader("X-Any") != "" {
		t.Fatalf("header=%q", c.GetHeader("X-Any"))
	}
	if _, err := c.Cookie("session"); !errors.Is(err, http.ErrNoCookie) {
		t.Fatalf("cookie err=%v", err)
	}
	if cookies := c.Cookies(); cookies != nil {
		t.Fatalf("cookies=%v", cookies)
	}

	body, err := c.BodyBytes()
	mustDo(t, err)
	if body != nil {
		t.Fatalf("body=%v", body)
	}
	bodyText, err := c.BodyString()
	mustDo(t, err)
	if bodyText != "" {
		t.Fatalf("body string=%q", bodyText)
	}

	if c.Scheme() != "http" {
		t.Fatalf("scheme=%q", c.Scheme())
	}
	if c.IP() != "" {
		t.Fatalf("ip=%q", c.IP())
	}
	if c.IPs() != nil {
		t.Fatalf("ips=%v", c.IPs())
	}
	if c.RemoteIP() != "" {
		t.Fatalf("remote ip=%q", c.RemoteIP())
	}
	if c.RequestID() != "" {
		t.Fatalf("request id=%q", c.RequestID())
	}

	if err := c.Next(); err != nil {
		t.Fatalf("next err=%v", err)
	}

	c.Set("key", "value")
	if got, ok := c.Get("key"); !ok || got != "value" {
		t.Fatalf("value=%v ok=%v", got, ok)
	}

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected panic from MustGet missing key")
		}
	}()
	_ = c.MustGet("missing")
}

func TestContextReleaseNilPointer(t *testing.T) {
	var c *Context
	c.release()
}

func TestContextBodyBytesLimitAndCloseErrors(t *testing.T) {
	t.Run("body limit exceeded", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("abcdef"))
		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()
		ctx.app = NewWithConfig(Config{BodyLimit: 3})

		body, err := ctx.BodyBytes()
		if !errors.Is(err, ErrRequestEntityTooLarge) {
			t.Fatalf("err=%v", err)
		}
		if body != nil {
			t.Fatalf("body=%v", body)
		}
	})

	t.Run("close error is propagated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = &fixedBody{data: []byte("ok"), closeErr: errors.New("close failed")}

		ctx, _ := newRecorderContext(t, req)
		defer ctx.release()

		body, err := ctx.BodyBytes()
		if err == nil || !strings.Contains(err.Error(), "close failed") {
			t.Fatalf("err=%v", err)
		}
		if body != nil {
			t.Fatalf("body=%v", body)
		}

		_, err = ctx.BodyBytes()
		if err == nil {
			t.Fatal("expected cached error on second BodyBytes call")
		}
	})
}

func TestReadAllBodyContentLengthHint(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		contentLength int64
	}{
		{name: "exact", body: "abc", contentLength: 3},
		{name: "shorter than declared", body: "abc", contentLength: 5},
		{name: "longer than declared", body: "abc", contentLength: 2},
		{name: "unknown", body: "abc", contentLength: -1},
		{name: "empty", body: "", contentLength: 1},
		{name: "hint above preallocation limit", body: "abc", contentLength: bodyReadPreallocateLimit + 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := readAllBody(strings.NewReader(test.body), test.contentLength)
			mustDo(t, err)
			if string(body) != test.body {
				t.Fatalf("body=%q want=%q", body, test.body)
			}
		})
	}
}

func TestContextProxyAndSchemeBranches(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set(HeaderXForwardedFor, "203.0.113.9, 10.0.0.1")
	req.Header.Set("X-Forwarded-Proto", "https, http")
	ctx, _ := newRecorderContext(t, req)
	defer ctx.release()
	ctx.app = NewWithConfig(Config{
		ProxyHeader:    HeaderXForwardedFor,
		TrustedProxies: []string{"10.0.0.1", "10.0.0.0/24"},
	})

	if !ctx.trustProxy() {
		t.Fatal("expected trusted proxy")
	}
	if scheme := ctx.Scheme(); scheme != "https" {
		t.Fatalf("scheme=%q", scheme)
	}
	ips := ctx.IPs()
	if len(ips) != 2 || ips[0] != "203.0.113.9" || ips[1] != "10.0.0.1" {
		t.Fatalf("ips=%v", ips)
	}
	if ip := ctx.IP(); ip != "203.0.113.9" {
		t.Fatalf("ip=%q", ip)
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://example.com/path", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	req2.Header.Set("X-Forwarded-Proto", "https")
	ctx2, _ := newRecorderContext(t, req2)
	defer ctx2.release()
	ctx2.app = NewWithConfig(Config{TrustedProxies: []string{"192.0.2.1"}})
	if ctx2.trustProxy() {
		t.Fatal("proxy should not be trusted")
	}
	if scheme := ctx2.Scheme(); scheme != "http" {
		t.Fatalf("scheme=%q", scheme)
	}

	req2.RemoteAddr = "malformed"
	if remote := ctx2.RemoteIP(); remote != "malformed" {
		t.Fatalf("remote ip=%q", remote)
	}
}

func TestContextParamMutationHelpers(t *testing.T) {
	ctx := &Context{}
	ctx.setParam("a", "1")
	ctx.setParam("b", "2")
	ctx.setParam("c", "3")

	route := &radixRoute{
		paramCount:       2,
		inlineParamNames: [2]string{"x", "y"},
	}
	ranges := paramRanges{}
	ranges.set(0, paramRange{start: 7, end: 9})
	ranges.set(1, paramRange{start: 16, end: 18})
	ctx.applyRouteParams("/users/10/posts/20", route, ranges)
	if ctx.paramCount != 2 {
		t.Fatalf("param count=%d", ctx.paramCount)
	}
	if ctx.Param("x") != "10" || ctx.Param("y") != "20" {
		t.Fatalf("params x=%q y=%q", ctx.Param("x"), ctx.Param("y"))
	}
	if got, ok := ctx.lookupPathParam("x"); !ok || got != "10" {
		t.Fatalf("lookup x=%q ok=%v", got, ok)
	}
	if ctx.PathParams[2] != emptyParam {
		t.Fatalf("stale path param=%+v", ctx.PathParams[2])
	}

	ctx.truncateParams(1)
	if ctx.paramCount != 1 {
		t.Fatalf("param count=%d", ctx.paramCount)
	}
	if ctx.Param("y") != "" {
		t.Fatalf("y should be cleared, got=%q", ctx.Param("y"))
	}
	if _, ok := ctx.lookupPathParam("y"); ok {
		t.Fatal("lookup for truncated param should fail")
	}

	ctx.truncateParams(-1)
	if ctx.paramCount != 0 {
		t.Fatalf("param count=%d", ctx.paramCount)
	}
}

func TestContextParamMutationHelpersBeyondInline(t *testing.T) {
	_, path, names, captured := buildSequentialParamRoute(10)

	route := &radixRoute{
		paramCount:       uint16(len(names)),
		inlineParamNames: [2]string{names[0], names[1]},
		extraParamNames:  append([]string(nil), names[2:]...),
	}

	ctx := &Context{}
	ctx.applyRouteParams(path, route, captured)
	if ctx.paramCount != len(names) {
		t.Fatalf("param count=%d", ctx.paramCount)
	}
	if got := ctx.Param(names[8]); got != "9" {
		t.Fatalf("param9=%q", got)
	}
	if got := ctx.Param(names[9]); got != "10" {
		t.Fatalf("param10=%q", got)
	}
	if got, ok := ctx.lookupPathParam(names[9]); !ok || got != "10" {
		t.Fatalf("lookup param10=%q ok=%v", got, ok)
	}
	if len(ctx.PathParams) < len(names) || ctx.PathParams[9].key != names[9] {
		t.Fatalf("path params not expanded: len=%d last=%+v", len(ctx.PathParams), ctx.PathParams[9])
	}

	ctx.truncateParams(8)
	if ctx.paramCount != 8 {
		t.Fatalf("param count=%d", ctx.paramCount)
	}
	if ctx.PathParams[8] != emptyParam || ctx.PathParams[9] != emptyParam {
		t.Fatalf("spill params should be cleared: p9=%+v p10=%+v", ctx.PathParams[8], ctx.PathParams[9])
	}
}

func TestIsTrustedProxyBranches(t *testing.T) {
	if isTrustedProxy("", []string{"10.0.0.1"}) {
		t.Fatal("empty ip should never be trusted")
	}
	if isTrustedProxy("not-an-ip", []string{"10.0.0.1"}) {
		t.Fatal("invalid ip should never be trusted")
	}
	if !isTrustedProxy("203.0.113.5", []string{"203.0.113.5"}) {
		t.Fatal("exact ip should be trusted")
	}
	if !isTrustedProxy("10.0.0.8", []string{"10.0.0.0/24"}) {
		t.Fatal("cidr should trust matching ip")
	}
	if isTrustedProxy("10.0.0.8", []string{"bad-cidr"}) {
		t.Fatal("invalid cidr should not trust ip")
	}
}

type fixedBody struct {
	data     []byte
	read     bool
	closeErr error
}

func (b *fixedBody) Read(p []byte) (int, error) {
	if b.read {
		return 0, io.EOF
	}
	b.read = true
	n := copy(p, b.data)
	return n, io.EOF
}

func (b *fixedBody) Close() error {
	return b.closeErr
}

// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0mjs/zinc"
)

func TestCSRFSafeRequestIssuesTokenCookieAndStoresState(t *testing.T) {
	app := zinc.New()
	app.Use(CSRFWithConfig(CSRFConfig{
		Generate: fixedCSRFToken("token"),
	}))
	app.Get("/form", func(c *zinc.Context) error {
		state := MustCSRFCurrent(c)
		if state.Token != "token" {
			t.Fatalf("token=%q", state.Token)
		}
		if !state.Issued {
			t.Fatal("token should be marked issued")
		}
		if state.Verified {
			t.Fatal("safe request should not be marked verified")
		}
		if state.CookieName != "_csrf" {
			t.Fatalf("cookie name=%q", state.CookieName)
		}
		if got := MustCSRFToken(c); got != "token" {
			t.Fatalf("must token=%q", got)
		}
		return c.String(state.Token)
	})

	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "token" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderSetCookie); !strings.Contains(got, "_csrf=token") {
		t.Fatalf("set-cookie=%q", got)
	}
	if got := rec.Header().Get(zinc.HeaderSetCookie); !strings.Contains(got, "Path=/") {
		t.Fatalf("set-cookie path=%q", got)
	}
	if got := rec.Header().Get(zinc.HeaderSetCookie); !strings.Contains(got, "SameSite=Lax") {
		t.Fatalf("set-cookie samesite=%q", got)
	}
	if vary := rec.Header().Values(zinc.HeaderVary); len(vary) != 1 || vary[0] != zinc.HeaderCookie {
		t.Fatalf("vary=%v", vary)
	}
}

func TestCSRFUnsafeRequestValidHeaderToken(t *testing.T) {
	app := zinc.New()
	app.Use(CSRFWithConfig(CSRFConfig{
		Generate: fixedCSRFToken("token"),
	}))
	app.Post("/submit", func(c *zinc.Context) error {
		state := MustCSRFCurrent(c)
		if state.Issued {
			t.Fatal("unsafe request should not issue a new token")
		}
		if !state.Verified {
			t.Fatal("unsafe request should be marked verified")
		}
		if state.FetchSite != "" {
			t.Fatalf("fetch site=%q", state.FetchSite)
		}
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: "token"})
	req.Header.Set(zinc.HeaderXCSRFToken, "token")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestCSRFMissingRequestTokenIsBadRequest(t *testing.T) {
	app := zinc.New()

	var got error
	app.Use(CSRFWithConfig(CSRFConfig{
		Generate: fixedCSRFToken("token"),
		ErrorHandler: func(_ *zinc.Context, err error) error {
			got = err
			return err
		},
	}))

	called := false
	app.Post("/submit", func(c *zinc.Context) error {
		called = true
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: "token"})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if called {
		t.Fatal("handler should not run")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !errors.Is(got, ErrCSRFTokenMissing) {
		t.Fatalf("error=%v", got)
	}
	if !errors.Is(got, zinc.ErrBadRequest) {
		t.Fatalf("expected bad request, got %v", got)
	}
}

func TestCSRFMissingCookieIsForbidden(t *testing.T) {
	app := zinc.New()

	var got error
	app.Use(CSRFWithConfig(CSRFConfig{
		ErrorHandler: func(_ *zinc.Context, err error) error {
			got = err
			return err
		},
	}))

	app.Post("/submit", func(c *zinc.Context) error {
		t.Fatal("handler should not run")
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Header.Set(zinc.HeaderXCSRFToken, "token")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !errors.Is(got, ErrCSRFCookieMissing) {
		t.Fatalf("error=%v", got)
	}
	if !errors.Is(got, zinc.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", got)
	}
}

func TestCSRFReadersFallBackToMultipartForm(t *testing.T) {
	app := zinc.New()
	app.Use(CSRFWithConfig(CSRFConfig{
		Generate: fixedCSRFToken("token"),
		Readers: []CSRFReader{
			CSRFFromHeader(zinc.HeaderXCSRFToken),
			CSRFFromForm("csrf"),
		},
	}))
	app.Post("/submit", func(c *zinc.Context) error {
		if got := c.FormValue("name"); got != "matt" {
			t.Fatalf("form name=%q", got)
		}
		if !MustCSRFCurrent(c).Verified {
			t.Fatal("request should be verified")
		}
		return c.String("ok")
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	mustNoErrCSRF(t, writer.WriteField("csrf", "token"))
	mustNoErrCSRF(t, writer.WriteField("name", "matt"))
	mustNoErrCSRF(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/submit", &body)
	req.Header.Set(zinc.HeaderContentType, writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: "token"})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestCSRFFromFirstEnforcesPrecedence(t *testing.T) {
	app := zinc.New()

	var got error
	app.Use(CSRFWithConfig(CSRFConfig{
		Generate: fixedCSRFToken("token"),
		Readers: []CSRFReader{
			CSRFFromFirst(
				CSRFFromHeader(zinc.HeaderXCSRFToken),
				CSRFFromForm("csrf"),
			),
		},
		ErrorHandler: func(_ *zinc.Context, err error) error {
			got = err
			return err
		},
	}))
	app.Post("/submit", func(c *zinc.Context) error {
		t.Fatal("handler should not run")
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader("csrf=token"))
	req.Header.Set(zinc.HeaderContentType, "application/x-www-form-urlencoded")
	req.Header.Set(zinc.HeaderXCSRFToken, "wrong")
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: "token"})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !errors.Is(got, ErrCSRFTokenInvalid) {
		t.Fatalf("error=%v", got)
	}
}

func TestCSRFFetchSiteBlocksCrossSite(t *testing.T) {
	app := zinc.New()

	var got error
	app.Use(CSRFWithConfig(CSRFConfig{
		ErrorHandler: func(_ *zinc.Context, err error) error {
			got = err
			return err
		},
	}))
	app.Post("/submit", func(c *zinc.Context) error {
		t.Fatal("handler should not run")
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: "token"})
	req.Header.Set(zinc.HeaderXCSRFToken, "token")
	req.Header.Set(zinc.HeaderSecFetchSite, string(CSRFFetchSiteCrossSite))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !errors.Is(got, ErrCSRFFetchSiteRejected) {
		t.Fatalf("error=%v", got)
	}
}

func TestCSRFFetchSiteTrustedOriginAllowsCrossSite(t *testing.T) {
	app := zinc.New()
	app.Use(CSRFWithConfig(CSRFConfig{
		TrustedOrigins: []string{"https://trusted.example.com"},
	}))
	app.Post("/submit", func(c *zinc.Context) error {
		if MustCSRFCurrent(c).FetchSite != CSRFFetchSiteCrossSite {
			t.Fatalf("fetch site=%q", MustCSRFCurrent(c).FetchSite)
		}
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: "token"})
	req.Header.Set(zinc.HeaderXCSRFToken, "token")
	req.Header.Set(zinc.HeaderSecFetchSite, string(CSRFFetchSiteCrossSite))
	req.Header.Set(zinc.HeaderOrigin, "https://trusted.example.com")
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestCSRFExposeHeader(t *testing.T) {
	app := zinc.New()
	app.Use(CSRFWithConfig(CSRFConfig{
		Generate:     fixedCSRFToken("token"),
		ExposeHeader: zinc.HeaderXCSRFToken,
	}))
	app.Get("/form", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if got := rec.Header().Get(zinc.HeaderXCSRFToken); got != "token" {
		t.Fatalf("exposed token=%q", got)
	}
}

func TestCSRFExistingSafeCookieIsReused(t *testing.T) {
	generateCalled := false
	app := zinc.New()
	app.Use(CSRFWithConfig(CSRFConfig{
		Generate: func(*zinc.Context) (string, error) {
			generateCalled = true
			return "new-token", nil
		},
	}))
	app.Get("/form", func(c *zinc.Context) error {
		state := MustCSRFCurrent(c)
		if state.Issued {
			t.Fatal("existing cookie should not be marked issued")
		}
		return c.String(state.Token)
	})

	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: " existing-token "})
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if generateCalled {
		t.Fatal("generator should not run when a token cookie already exists")
	}
	if rec.Body.String() != "existing-token" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderSetCookie); !strings.Contains(got, "_csrf=existing-token") {
		t.Fatalf("set-cookie=%q", got)
	}
}

func TestCSRFAllowFetchSiteDecider(t *testing.T) {
	t.Run("allows same site decision", func(t *testing.T) {
		var decision CSRFDecision
		app := zinc.New()
		app.Use(CSRFWithConfig(CSRFConfig{
			AllowFetchSite: func(_ *zinc.Context, got CSRFDecision) (bool, error) {
				decision = got
				return got.Site == CSRFFetchSiteSameSite, nil
			},
		}))
		app.Post("/submit", func(c *zinc.Context) error {
			return c.String("ok")
		})

		req := httptest.NewRequest(http.MethodPost, "/submit", nil)
		req.AddCookie(&http.Cookie{Name: "_csrf", Value: "token"})
		req.Header.Set(zinc.HeaderXCSRFToken, "token")
		req.Header.Set(zinc.HeaderSecFetchSite, " Same-Site ")
		req.Header.Set(zinc.HeaderOrigin, "https://app.example.com")
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
		}
		if decision.Site != CSRFFetchSiteSameSite || decision.Origin != "https://app.example.com" || decision.Trusted {
			t.Fatalf("decision=%+v", decision)
		}
	})

	t.Run("returns decider error", func(t *testing.T) {
		deciderErr := errors.New("policy unavailable")
		app := zinc.New()
		app.Use(CSRFWithConfig(CSRFConfig{
			AllowFetchSite: func(*zinc.Context, CSRFDecision) (bool, error) {
				return false, deciderErr
			},
			ErrorHandler: func(_ *zinc.Context, err error) error {
				if !errors.Is(err, deciderErr) {
					t.Fatalf("err=%v", err)
				}
				return err
			},
		}))
		app.Post("/submit", func(c *zinc.Context) error {
			t.Fatal("handler should not run")
			return nil
		})

		req := httptest.NewRequest(http.MethodPost, "/submit", nil)
		req.AddCookie(&http.Cookie{Name: "_csrf", Value: "token"})
		req.Header.Set(zinc.HeaderXCSRFToken, "token")
		req.Header.Set(zinc.HeaderSecFetchSite, string(CSRFFetchSiteCrossSite))
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
		}
	})
}

func TestCSRFSkipper(t *testing.T) {
	app := zinc.New()
	app.Use(CSRFWithConfig(CSRFConfig{
		Skipper: func(*zinc.Context) bool { return true },
	}))
	app.Get("/skip", func(c *zinc.Context) error {
		if _, ok := CSRFCurrent(c); ok {
			t.Fatal("state should not be present")
		}
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/skip", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderSetCookie); got != "" {
		t.Fatalf("set-cookie=%q", got)
	}
}

func TestCSRFViolationErrorMatching(t *testing.T) {
	missing := &CSRFViolation{
		Reason: CSRFReasonTokenMissing,
		Cause:  ErrCSRFTokenMissing,
	}
	if got := missing.Error(); !strings.Contains(got, ErrCSRFTokenMissing.Error()) {
		t.Fatalf("missing error=%q", got)
	}
	if !errors.Is(missing, ErrCSRFTokenMissing) {
		t.Fatal("missing token should match sentinel")
	}
	if !errors.Is(missing, zinc.ErrBadRequest) {
		t.Fatal("missing token should match bad request")
	}

	invalid := &CSRFViolation{Reason: CSRFReasonTokenInvalid}
	if got := invalid.Error(); got != ErrCSRFTokenInvalid.Error() {
		t.Fatalf("invalid error=%q", got)
	}
	if !errors.Is(invalid, ErrCSRFTokenInvalid) {
		t.Fatal("invalid token should match sentinel")
	}
	if !errors.Is(invalid, zinc.ErrForbidden) {
		t.Fatal("invalid token should match forbidden")
	}

	cookie := &CSRFViolation{Reason: CSRFReasonCookieMissing, CookieName: "_csrf"}
	if got := cookie.Error(); !strings.Contains(got, "_csrf") {
		t.Fatalf("cookie error=%q", got)
	}

	fetch := &CSRFViolation{Reason: CSRFReasonFetchSiteRejected, FetchSite: CSRFFetchSiteCrossSite}
	if got := fetch.Error(); !strings.Contains(got, string(CSRFFetchSiteCrossSite)) {
		t.Fatalf("fetch error=%q", got)
	}

	cause := &CSRFViolation{Cause: errors.New("custom")}
	if got := cause.Error(); got != "custom" {
		t.Fatalf("cause error=%q", got)
	}

	var nilViolation *CSRFViolation
	if got := nilViolation.Error(); got == "" {
		t.Fatal("nil violation should have a fallback message")
	}
}

func TestCSRFSameSiteNoneForcesSecure(t *testing.T) {
	app := zinc.New()
	app.Use(CSRFWithConfig(CSRFConfig{
		Generate: fixedCSRFToken("token"),
		Cookie: CSRFCookie{
			SameSite: http.SameSiteNoneMode,
		},
	}))
	app.Get("/form", func(c *zinc.Context) error {
		return c.String("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	setCookie := rec.Header().Get(zinc.HeaderSetCookie)
	if !strings.Contains(setCookie, "SameSite=None") {
		t.Fatalf("set-cookie=%q", setCookie)
	}
	if !strings.Contains(setCookie, "Secure") {
		t.Fatalf("set-cookie=%q", setCookie)
	}
}

func TestCSRFConfigValidation(t *testing.T) {
	assertPanicCSRF(t, func() {
		_ = CSRFWithConfig(CSRFConfig{Readers: []CSRFReader{nil}})
	})
	assertPanicCSRF(t, func() {
		_ = CSRFWithConfig(CSRFConfig{TrustedOrigins: []string{"https://example.com/path"}})
	})
	assertPanicCSRF(t, func() {
		_ = CSRFWithConfig(CSRFConfig{TrustedOrigins: []string{"://bad"}})
	})
}

func fixedCSRFToken(token string) CSRFGenerator {
	return func(*zinc.Context) (string, error) {
		return token, nil
	}
}

func mustNoErrCSRF(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertPanicCSRF(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

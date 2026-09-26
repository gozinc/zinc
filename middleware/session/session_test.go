package session

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0mjs/zinc"
)

func sessionTestRequest(app http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	app.ServeHTTP(w, r)
	return w
}

func sessionTestApp(cfg Config) *zinc.App {
	app := zinc.New()
	app.Use(New(cfg))
	app.Get("/login", func(c *zinc.Context) error {
		if err := MustGet(c).Set("user", "alice"); err != nil {
			return err
		}
		return c.String("ok")
	})
	app.Get("/who", func(c *zinc.Context) error { return c.String(MustGet(c).Get("user")) })
	return app
}

func TestSessionAuthenticatedExpiryAndRotation(t *testing.T) {
	now := time.Unix(1700000000, 0)
	oldKey, newKey := []byte(strings.Repeat("o", 32)), []byte(strings.Repeat("n", 32))
	cfg := Config{Name: "sid", Secret: oldKey, MaxAge: 100, Now: func() time.Time { return now }}
	oldApp := sessionTestApp(cfg)
	oldCookie := sessionTestRequest(oldApp, "/login", nil).Result().Cookies()[0]
	now = now.Add(10 * time.Second)
	cfg.Secret, cfg.PreviousSecrets = newKey, [][]byte{oldKey}
	rotation := sessionTestRequest(sessionTestApp(cfg), "/who", oldCookie)
	if rotation.Body.String() != "alice" {
		t.Fatalf("rotation: %d %q", rotation.Code, rotation.Body.String())
	}
	rotated := rotation.Result().Cookies()[0]
	if !rotated.Expires.Equal(oldCookie.Expires) {
		t.Fatal("rotation extended authenticated expiry")
	}
	cfg.PreviousSecrets = nil
	newApp := sessionTestApp(cfg)
	if w := sessionTestRequest(newApp, "/who", oldCookie); w.Code != 400 {
		t.Fatal("retired signing key still accepted")
	}
	if w := sessionTestRequest(newApp, "/who", rotated); w.Body.String() != "alice" {
		t.Fatal("rotated cookie not signed with current key")
	}
	now = now.Add(90 * time.Second)
	if w := sessionTestRequest(newApp, "/who", rotated); w.Code != 400 {
		t.Fatal("expired signed session accepted")
	}
}

func TestBrowserSessionCookieStillHasServerExpiry(t *testing.T) {
	now := time.Unix(1700000000, 0)
	cfg := Config{Secret: []byte(strings.Repeat("s", 32)), Lifetime: 2 * time.Second, Now: func() time.Time { return now }}
	app := sessionTestApp(cfg)
	cookie := sessionTestRequest(app, "/login", nil).Result().Cookies()[0]
	if cookie.MaxAge != 0 || !cookie.Expires.IsZero() {
		t.Fatal("browser session cookie attributes changed")
	}
	now = now.Add(2 * time.Second)
	w := sessionTestRequest(app, "/who", cookie)
	if w.Code != 400 {
		t.Fatal("browser session cookie has no server expiry")
	}
	if cookies := w.Result().Cookies(); len(cookies) != 1 || cookies[0].MaxAge >= 0 {
		t.Fatal("expired browser cookie was not cleared")
	}
}

func TestSessionRejectsLegacyUnsignedExpiryAndOversizedInput(t *testing.T) {
	key := []byte(strings.Repeat("s", 32))
	app := sessionTestApp(Config{Secret: key})
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"user":"alice"}`))
	for _, value := range []string{payload + "." + signSessionPayload(payload, key), strings.Repeat("x", 4097)} {
		if w := sessionTestRequest(app, "/who", &http.Cookie{Name: "zinc_session", Value: value}); w.Code != 400 {
			t.Fatalf("invalid session accepted: %d", w.Code)
		}
	}
}

func TestSessionMutationBeforeAndAfterCommit(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Name: "sid", Secret: []byte(strings.Repeat("s", 32))}))
	app.Get("/", func(c *zinc.Context) error {
		c.SetCookie(&http.Cookie{Name: "unrelated", Value: "keep", Path: "/"})
		s := MustGet(c)
		if err := s.Set("user", "alice"); err != nil {
			return err
		}
		if err := s.Set("too-large", strings.Repeat("x", 8192)); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("oversized mutation: %v", err)
		}
		if s.Get("too-large") != "" {
			t.Fatal("failed mutation changed session")
		}
		if err := s.Set("user", "bob"); err != nil {
			return err
		}
		if err := c.String("committed"); err != nil {
			return err
		}
		if err := s.Set("user", "mallory"); !errors.Is(err, ErrCommitted) {
			t.Fatalf("late mutation: %v", err)
		}
		if s.Get("user") != "bob" {
			t.Fatal("late mutation changed session")
		}
		return nil
	})
	w := sessionTestRequest(app, "/", nil)
	if w.Body.String() != "committed" {
		t.Fatalf("body=%q", w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 2 || cookies[0].Name != "unrelated" {
		t.Fatalf("cookies=%v", cookies)
	}
}

func TestSessionSSEFlushesBeforeHandlerReturns(t *testing.T) {
	app := zinc.New()
	app.Use(New(Config{Name: "sid", Secret: []byte(strings.Repeat("s", 32))}))
	w := httptest.NewRecorder()
	app.Get("/", func(c *zinc.Context) error {
		if err := MustGet(c).Set("user", "alice"); err != nil {
			return err
		}
		if err := c.SSE(zinc.Event{Data: "hello"}); err != nil {
			return err
		}
		if err := http.NewResponseController(c.Writer()).Flush(); err != nil {
			return err
		}
		if !strings.Contains(w.Body.String(), "hello") {
			t.Fatal("session buffered stream")
		}
		return nil
	})
	app.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
}

func TestSessionRejectsWeakSigningKeys(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("weak signing key accepted")
		}
	}()
	New(Config{Name: "sid", Secret: []byte("short")})
}

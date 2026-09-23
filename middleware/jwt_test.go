// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: 2024-present Matt J. Stevenson and Contributors

package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0mjs/zinc"
	jwtgo "github.com/golang-jwt/jwt/v5"
)

func TestJWTMiddlewareWithMapClaims(t *testing.T) {
	app := zinc.New()
	app.Use(JWTWithConfig(JWTConfig{
		KeyFunc: func(*zinc.Context, *jwtgo.Token) (any, error) {
			return []byte("secret"), nil
		},
		ParserOptions: []jwtgo.ParserOption{
			jwtgo.WithValidMethods([]string{jwtgo.SigningMethodHS256.Alg()}),
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		token := MustJWTToken(c)
		claims := MustJWTClaims[jwtgo.MapClaims](c)
		return c.JSON(zinc.Map{
			"alg": token.Method.Alg(),
			"sub": claims["sub"],
			"raw": MustJWTTokenString(c),
		})
	})

	tokenString := mustSignedStringJWT(t, jwtgo.MapClaims{
		"sub": "user-123",
	}, []byte("secret"))

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if body != `{"alg":"HS256","raw":"`+tokenString+`","sub":"user-123"}`+"\n" {
		t.Fatalf("body=%q", body)
	}
}

func TestJWTMiddlewareWithTypedClaims(t *testing.T) {
	type claims struct {
		Role string `json:"role"`
		jwtgo.RegisteredClaims
	}

	app := zinc.New()
	app.Use(JWTWithConfig(JWTConfig{
		KeyFunc: func(*zinc.Context, *jwtgo.Token) (any, error) {
			return []byte("secret"), nil
		},
		NewClaims: func(*zinc.Context) jwtgo.Claims {
			return &claims{}
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		got := MustJWTClaims[*claims](c)
		return c.String(got.Role + ":" + got.Subject)
	})

	tokenString := mustSignedStringJWT(t, &claims{
		Role: "admin",
		RegisteredClaims: jwtgo.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwtgo.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}, []byte("secret"))

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "admin:user-1" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestJWTMiddlewareMissingToken(t *testing.T) {
	app := zinc.New()
	app.Use(JWTWithConfig(JWTConfig{
		KeyFunc: func(*zinc.Context, *jwtgo.Token) (any, error) {
			return []byte("secret"), nil
		},
		Realm: "api",
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Body.String() != http.StatusText(http.StatusUnauthorized) {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if got := rec.Header().Get(zinc.HeaderWWWAuthenticate); got != `Bearer realm="api"` {
		t.Fatalf("challenge=%q", got)
	}
}

func TestJWTMiddlewareMalformedHeader(t *testing.T) {
	app := zinc.New()
	app.Use(JWTWithConfig(JWTConfig{
		KeyFunc: func(*zinc.Context, *jwtgo.Token) (any, error) {
			return []byte("secret"), nil
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Token abc")
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get(zinc.HeaderWWWAuthenticate); got != `Bearer error="invalid_request"` {
		t.Fatalf("challenge=%q", got)
	}
}

func TestJWTMiddlewareInvalidToken(t *testing.T) {
	app := zinc.New()
	app.Use(JWTWithConfig(JWTConfig{
		KeyFunc: func(*zinc.Context, *jwtgo.Token) (any, error) {
			return []byte("secret"), nil
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	tokenString := mustSignedStringJWT(t, jwtgo.MapClaims{
		"sub": "user-123",
	}, []byte("wrong-secret"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Bearer "+tokenString)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get(zinc.HeaderWWWAuthenticate); got != `Bearer error="invalid_token"` {
		t.Fatalf("challenge=%q", got)
	}
}

func TestJWTMiddlewareSkipper(t *testing.T) {
	app := zinc.New()
	app.Use(JWTWithConfig(JWTConfig{
		Skipper: func(*zinc.Context) bool { return true },
		KeyFunc: func(*zinc.Context, *jwtgo.Token) (any, error) {
			return []byte("secret"), nil
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		if _, ok := JWTToken(c); ok {
			t.Fatal("token should not be present")
		}
		return c.String("ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestJWTMiddlewareFromFirst(t *testing.T) {
	app := zinc.New()
	app.Use(JWTWithConfig(JWTConfig{
		Extractor: JWTFromFirst(
			JWTFromAuthHeader("Bearer"),
			JWTFromCookie("access_token"),
		),
		KeyFunc: func(*zinc.Context, *jwtgo.Token) (any, error) {
			return []byte("secret"), nil
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String(MustJWTTokenString(c))
	})

	tokenString := mustSignedStringJWT(t, jwtgo.MapClaims{
		"sub": "cookie-user",
	}, []byte("secret"))

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: tokenString})
	rec := httptest.NewRecorder()

	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != tokenString {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestJWTMiddlewareValidateCanRejectWithForbidden(t *testing.T) {
	app := zinc.New()
	app.Use(JWTWithConfig(JWTConfig{
		KeyFunc: func(*zinc.Context, *jwtgo.Token) (any, error) {
			return []byte("secret"), nil
		},
		Validate: func(_ *zinc.Context, token *jwtgo.Token) error {
			claims, ok := token.Claims.(jwtgo.MapClaims)
			if !ok || claims["role"] != "admin" {
				return zinc.ErrForbidden
			}
			return nil
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	tokenString := mustSignedStringJWT(t, jwtgo.MapClaims{
		"role": "member",
	}, []byte("secret"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Bearer "+tokenString)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}
	if got := rec.Header().Get(zinc.HeaderWWWAuthenticate); got != "" {
		t.Fatalf("challenge=%q", got)
	}
}

func TestJWTMiddlewareCustomErrorHandler(t *testing.T) {
	var gotErr error

	app := zinc.New()
	app.Use(JWTWithConfig(JWTConfig{
		KeyFunc: func(*zinc.Context, *jwtgo.Token) (any, error) {
			return []byte("secret"), nil
		},
		ErrorHandler: func(c *zinc.Context, err error) error {
			gotErr = err
			return c.Status(http.StatusTeapot).String("bad token")
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		return c.String("ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Body.String() != "bad token" {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if !errors.Is(gotErr, ErrJWTTokenMissing) {
		t.Fatalf("err=%v", gotErr)
	}
}

func TestJWTMiddlewareParseTokenFunc(t *testing.T) {
	app := zinc.New()
	app.Use(JWTWithConfig(JWTConfig{
		ParseTokenFunc: func(_ *zinc.Context, tokenString string) (*jwtgo.Token, error) {
			return &jwtgo.Token{
				Valid:  true,
				Method: jwtgo.SigningMethodHS256,
				Claims: jwtgo.MapClaims{"name": tokenString},
			}, nil
		},
	}))
	app.Get("/private", func(c *zinc.Context) error {
		got := MustJWTClaims[jwtgo.MapClaims](c)
		return c.String(got["name"].(string))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private?token=alice", nil)
	req.Header.Set(zinc.HeaderAuthorization, "Bearer alice")
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "alice" {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestJWTMiddlewarePanicsWithoutParserOrKeyFunc(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	_ = JWTWithConfig(JWTConfig{})
}

func TestJWTMustAccessorsPanicWhenMissing(t *testing.T) {
	c := zinc.NewContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	assertPanicsJWT(t, func() {
		_ = MustJWTToken(c)
	})
	assertPanicsJWT(t, func() {
		_ = MustJWTTokenString(c)
	})
	assertPanicsJWT(t, func() {
		_ = MustJWTClaims[jwtgo.MapClaims](c)
	})
}

func mustSignedStringJWT(t *testing.T, claims jwtgo.Claims, key []byte) string {
	t.Helper()
	token := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("signed string error: %v", err)
	}
	return tokenString
}

func mustNoErrJWT(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertPanicsJWT(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

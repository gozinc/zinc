package zinc_test

import (
	"net/http/httptest"
	"testing"

	"github.com/0mjs/zinc"
)

func TestProxyTrustWalkAndConfigOwnership(t *testing.T) {
	cfg := zinc.Config{}
	cfg.TrustedProxies = []string{"10.0.0.0/8", "2001:db8::/32"}
	app := zinc.New(cfg)
	cfg.TrustedProxies[0] = "0.0.0.0/0"
	app.Get("/", func(c *zinc.Context) error { return c.Send(c.IP() + "|" + c.Scheme()) })
	for _, tt := range []struct{ peer, chain, proto, want string }{
		{"10.0.0.1:1234", "198.51.100.7, 203.0.113.9", "https", "203.0.113.9|https"},
		{"10.0.0.1:1234", "203.0.113.9, 10.1.1.1", "https, http", "203.0.113.9|http"},
		{"10.0.0.1:1234", "203.0.113.9, garbage", "ftp", "10.0.0.1|http"},
		{"192.0.2.1:1234", "203.0.113.9", "https", "192.0.2.1|http"},
		{"[2001:db8::1]:1234", "2001:db9::1, 2001:db8::2", "https", "2001:db9::1|https"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.peer
			r.Header.Set("X-Forwarded-For", tt.chain)
			r.Header.Set("X-Forwarded-Proto", tt.proto)
			w := httptest.NewRecorder()
			app.ServeHTTP(w, r)
			if w.Body.String() != tt.want {
				t.Fatalf("got %q want %q", w.Body.String(), tt.want)
			}
		})
	}
	defer func() {
		if recover() == nil {
			t.Fatal("invalid trust configuration accepted")
		}
	}()
	zinc.New(zinc.Config{TrustedProxies: []string{"typo/24"}})
}

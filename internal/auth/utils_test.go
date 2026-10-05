package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	expect "github.com/yusing/goutils/testing"
)

func TestCookieDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		host   string
		remote string
		want   string
	}{
		{host: "app.example.com", want: ".example.com"},
		{host: "a.b.example.com", want: ".b.example.com"},
		{host: "example.com", want: ""},
		{host: "Example.COM", want: ""},
		{host: "example.host", want: ""},
		{host: "app.example.host", want: ".example.host"},
		{host: "example.co.uk", want: ""},
		{host: "foo.example.co.uk", want: ".example.co.uk"},
		{host: "foo.blogspot.com", want: ""},
		{host: "a.foo.blogspot.com", want: ".foo.blogspot.com"},
		{host: "app.localhost", want: ""},
		{host: "a.b.localhost", want: ".b.localhost"},
		{host: "app.local", want: ""},
		{host: "a.b.local", want: ".b.local"},
		{host: "app.internal", want: ""},
		{host: "localhost", want: ""},
		{host: "192.0.2.10", want: ""},
		{host: "192.0.2.10:8080", want: ""},
		{host: "[2001:db8::1]", want: ""},
		{host: "[2001:db8::1]:8443", want: ""},
		{host: "app.example.com:8443", want: ".example.com"},
		{host: "app.example.host:443", want: ".example.host"},
		{host: "app.example.com.", want: ".example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			expect.Equal(t, cookieDomain(req), tt.want)
		})
	}

	t.Run("forwarded public suffix", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Host = "127.0.0.1:8080"
		req.Header.Set("X-Forwarded-Host", "example.host")
		expect.Equal(t, cookieDomain(req), "")
	})

	t.Run("forwarded host with port and proxy list", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Host = "127.0.0.1:8080"
		req.Header.Set("X-Forwarded-Host", "app.example.com:8443, proxy.internal")
		expect.Equal(t, cookieDomain(req), ".example.com")
	})
}

func TestSetTokenCookieSkipsPublicSuffixDomain(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.host"
	SetTokenCookie(rec, req, "godoxy_token", "session", time.Hour)

	raw := rec.Header().Get("Set-Cookie")
	expect.True(t, !strings.Contains(raw, "Domain="))
	cookie := expect.Must(http.ParseSetCookie(raw))
	expect.Equal(t, cookie.Domain, "")
	expect.Equal(t, cookie.Name, "godoxy_token")
	expect.Equal(t, cookie.Value, "session")
	expect.Equal(t, cookie.Path, "/")
}

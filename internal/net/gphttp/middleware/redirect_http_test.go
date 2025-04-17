package middleware

import (
	"net/http"
	"net/url"
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestRedirectToHTTPs(t *testing.T) {
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: expect.Must(url.Parse("http://example.com")),
	})
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusPermanentRedirect)
	expect.Equal(t, result.ResponseHeaders.Get("Location"), "https://example.com")
}

func TestNoRedirect(t *testing.T) {
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: expect.Must(url.Parse("https://example.com")),
	})
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusOK)
}

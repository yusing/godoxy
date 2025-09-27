package middleware

import (
	"net/http"
	"testing"

	nettypes "github.com/yusing/godoxy/internal/net/types"
	expect "github.com/yusing/goutils/testing"
)

func TestRedirectToHTTPs(t *testing.T) {
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: nettypes.MustParseURL("http://example.com"),
	})
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusPermanentRedirect)
	expect.Equal(t, result.ResponseHeaders.Get("Location"), "https://example.com")
}

func TestNoRedirect(t *testing.T) {
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: nettypes.MustParseURL("https://example.com"),
	})
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusOK)
}

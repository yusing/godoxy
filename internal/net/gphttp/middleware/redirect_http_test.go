package middleware

import (
	"net/http"
	"testing"

	nettypes "github.com/yusing/go-proxy/internal/net/types"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestRedirectToHTTPs(t *testing.T) {
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: nettypes.MustParseURL("http://example.com"),
	})
	ExpectNoError(t, err)
	ExpectEqual(t, result.ResponseStatus, http.StatusPermanentRedirect)
	ExpectEqual(t, result.ResponseHeaders.Get("Location"), "https://example.com")
}

func TestNoRedirect(t *testing.T) {
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: nettypes.MustParseURL("https://example.com"),
	})
	ExpectNoError(t, err)
	ExpectEqual(t, result.ResponseStatus, http.StatusOK)
}

package middleware

import (
	"net/http"
	"net/url"
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestRedirectToHTTPs(t *testing.T) {
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: Must(url.Parse("http://example.com")),
	})
	ExpectNoError(t, err)
	ExpectEqual(t, result.ResponseStatus, http.StatusPermanentRedirect)
	ExpectEqual(t, result.ResponseHeaders.Get("Location"), "https://example.com")
}

func TestNoRedirect(t *testing.T) {
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: Must(url.Parse("https://example.com")),
	})
	ExpectNoError(t, err)
	ExpectEqual(t, result.ResponseStatus, http.StatusOK)
}

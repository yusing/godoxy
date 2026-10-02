package middleware

import (
	"net/http"
	"testing"

	"github.com/yusing/godoxy/internal/common"
	expect "github.com/yusing/goutils/testing"
)

func TestRedirectToHTTPs(t *testing.T) {
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: mustParseURL("http://example.com"),
	})
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusPermanentRedirect)
	expect.Equal(t, result.ResponseHeaders.Get("Location"), "https://example.com")
}

func TestNoRedirect(t *testing.T) {
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: mustParseURL("https://example.com"),
	})
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusOK)
}

func TestNoRedirectWhenSharedHTTPSDisabled(t *testing.T) {
	previous := common.ProxyHTTPSAddr
	common.ProxyHTTPSAddr = ""
	t.Cleanup(func() { common.ProxyHTTPSAddr = previous })
	result, err := newMiddlewareTest(RedirectHTTP, &testArgs{
		reqURL: mustParseURL("http://example.com"),
	})
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusOK)
	expect.Equal(t, result.ResponseHeaders.Get("Location"), "")
}

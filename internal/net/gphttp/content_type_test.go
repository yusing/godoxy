package gphttp

import (
	"net/http"
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestContentTypes(t *testing.T) {
	expect.True(t, GetContentType(http.Header{"Content-Type": {"text/html"}}).IsHTML())
	expect.True(t, GetContentType(http.Header{"Content-Type": {"text/html; charset=utf-8"}}).IsHTML())
	expect.True(t, GetContentType(http.Header{"Content-Type": {"application/xhtml+xml"}}).IsHTML())
	expect.False(t, GetContentType(http.Header{"Content-Type": {"text/plain"}}).IsHTML())

	expect.True(t, GetContentType(http.Header{"Content-Type": {"application/json"}}).IsJSON())
	expect.True(t, GetContentType(http.Header{"Content-Type": {"application/json; charset=utf-8"}}).IsJSON())
	expect.False(t, GetContentType(http.Header{"Content-Type": {"text/html"}}).IsJSON())

	expect.True(t, GetContentType(http.Header{"Content-Type": {"text/plain"}}).IsPlainText())
	expect.True(t, GetContentType(http.Header{"Content-Type": {"text/plain; charset=utf-8"}}).IsPlainText())
	expect.False(t, GetContentType(http.Header{"Content-Type": {"text/html"}}).IsPlainText())
}

func TestAcceptContentTypes(t *testing.T) {
	expect.True(t, GetAccept(http.Header{"Accept": {"text/html", "text/plain"}}).AcceptPlainText())
	expect.True(t, GetAccept(http.Header{"Accept": {"text/html", "text/plain; charset=utf-8"}}).AcceptPlainText())
	expect.True(t, GetAccept(http.Header{"Accept": {"text/html", "text/plain"}}).AcceptHTML())
	expect.True(t, GetAccept(http.Header{"Accept": {"application/json"}}).AcceptJSON())
	expect.True(t, GetAccept(http.Header{"Accept": {"*/*"}}).AcceptPlainText())
	expect.True(t, GetAccept(http.Header{"Accept": {"*/*"}}).AcceptHTML())
	expect.True(t, GetAccept(http.Header{"Accept": {"*/*"}}).AcceptJSON())
	expect.True(t, GetAccept(http.Header{"Accept": {"text/*"}}).AcceptPlainText())
	expect.True(t, GetAccept(http.Header{"Accept": {"text/*"}}).AcceptHTML())

	expect.False(t, GetAccept(http.Header{"Accept": {"text/plain"}}).AcceptHTML())
	expect.False(t, GetAccept(http.Header{"Accept": {"text/plain; charset=utf-8"}}).AcceptHTML())
	expect.False(t, GetAccept(http.Header{"Accept": {"text/html"}}).AcceptPlainText())
	expect.False(t, GetAccept(http.Header{"Accept": {"text/html"}}).AcceptJSON())
	expect.False(t, GetAccept(http.Header{"Accept": {"text/*"}}).AcceptJSON())
}

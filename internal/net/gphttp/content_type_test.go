package gphttp

import (
	"net/http"
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestContentTypes(t *testing.T) {
	ExpectTrue(t, GetContentType(http.Header{"Content-Type": {"text/html"}}).IsHTML())
	ExpectTrue(t, GetContentType(http.Header{"Content-Type": {"text/html; charset=utf-8"}}).IsHTML())
	ExpectTrue(t, GetContentType(http.Header{"Content-Type": {"application/xhtml+xml"}}).IsHTML())
	ExpectFalse(t, GetContentType(http.Header{"Content-Type": {"text/plain"}}).IsHTML())

	ExpectTrue(t, GetContentType(http.Header{"Content-Type": {"application/json"}}).IsJSON())
	ExpectTrue(t, GetContentType(http.Header{"Content-Type": {"application/json; charset=utf-8"}}).IsJSON())
	ExpectFalse(t, GetContentType(http.Header{"Content-Type": {"text/html"}}).IsJSON())

	ExpectTrue(t, GetContentType(http.Header{"Content-Type": {"text/plain"}}).IsPlainText())
	ExpectTrue(t, GetContentType(http.Header{"Content-Type": {"text/plain; charset=utf-8"}}).IsPlainText())
	ExpectFalse(t, GetContentType(http.Header{"Content-Type": {"text/html"}}).IsPlainText())
}

func TestAcceptContentTypes(t *testing.T) {
	ExpectTrue(t, GetAccept(http.Header{"Accept": {"text/html", "text/plain"}}).AcceptPlainText())
	ExpectTrue(t, GetAccept(http.Header{"Accept": {"text/html", "text/plain; charset=utf-8"}}).AcceptPlainText())
	ExpectTrue(t, GetAccept(http.Header{"Accept": {"text/html", "text/plain"}}).AcceptHTML())
	ExpectTrue(t, GetAccept(http.Header{"Accept": {"application/json"}}).AcceptJSON())
	ExpectTrue(t, GetAccept(http.Header{"Accept": {"*/*"}}).AcceptPlainText())
	ExpectTrue(t, GetAccept(http.Header{"Accept": {"*/*"}}).AcceptHTML())
	ExpectTrue(t, GetAccept(http.Header{"Accept": {"*/*"}}).AcceptJSON())
	ExpectTrue(t, GetAccept(http.Header{"Accept": {"text/*"}}).AcceptPlainText())
	ExpectTrue(t, GetAccept(http.Header{"Accept": {"text/*"}}).AcceptHTML())

	ExpectFalse(t, GetAccept(http.Header{"Accept": {"text/plain"}}).AcceptHTML())
	ExpectFalse(t, GetAccept(http.Header{"Accept": {"text/plain; charset=utf-8"}}).AcceptHTML())
	ExpectFalse(t, GetAccept(http.Header{"Accept": {"text/html"}}).AcceptPlainText())
	ExpectFalse(t, GetAccept(http.Header{"Accept": {"text/html"}}).AcceptJSON())
	ExpectFalse(t, GetAccept(http.Header{"Accept": {"text/*"}}).AcceptJSON())
}

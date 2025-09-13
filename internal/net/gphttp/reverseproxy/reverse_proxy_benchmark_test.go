package reverseproxy

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	nettypes "github.com/yusing/go-proxy/internal/net/types"
)

type noopTransport struct{}

func (t noopTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Body:          io.NopCloser(strings.NewReader("Hello, world!")),
		Request:       req,
		ContentLength: int64(len("Hello, world!")),
		Header:        http.Header{},
	}, nil
}

type noopResponseWriter struct{}

func (w noopResponseWriter) Header() http.Header {
	return http.Header{}
}

func (w noopResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (w noopResponseWriter) WriteHeader(statusCode int) {
}

func BenchmarkReverseProxy(b *testing.B) {
	var w noopResponseWriter
	var req = http.Request{
		Method: "GET",
		URL:    &url.URL{Scheme: "http", Host: "test"},
		Body:   io.NopCloser(strings.NewReader("Hello, world!")),
	}
	proxy := NewReverseProxy("test", nettypes.MustParseURL("http://localhost:8080"), noopTransport{})
	for b.Loop() {
		proxy.ServeHTTP(w, &req)
	}
}

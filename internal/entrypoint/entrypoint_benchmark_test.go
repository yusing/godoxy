package entrypoint

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/yusing/go-proxy/internal/route"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/types"
)

type noopResponseWriter struct {
	statusCode int
	written    []byte
}

func (w *noopResponseWriter) Header() http.Header {
	return http.Header{}
}
func (w *noopResponseWriter) Write(b []byte) (int, error) {
	w.written = b
	return len(b), nil
}
func (w *noopResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

type noopTransport struct{}

func (t noopTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("1")),
		Request:    req,
		Header:     http.Header{},
	}, nil
}

func BenchmarkEntrypointReal(b *testing.B) {
	var ep Entrypoint
	var req = http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/", RawPath: "/"},
		Host:   "test.domain.tld",
	}
	ep.SetFindRouteDomains([]string{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1")
		w.Write([]byte("1"))
	}))
	defer srv.Close()

	url, err := url.Parse(srv.URL)
	if err != nil {
		b.Fatal(err)
	}

	host, port, err := net.SplitHostPort(url.Host)
	if err != nil {
		b.Fatal(err)
	}

	portInt, err := strconv.Atoi(port)
	if err != nil {
		b.Fatal(err)
	}

	r := &route.Route{
		Alias:       "test",
		Scheme:      "http",
		Host:        host,
		Port:        route.Port{Proxy: portInt},
		HealthCheck: &types.HealthCheckConfig{Disable: true},
	}

	err = r.Validate()
	if err != nil {
		b.Fatal(err)
	}

	err = r.Start(task.RootTask("test", false))
	if err != nil {
		b.Fatal(err)
	}

	var w noopResponseWriter

	b.ResetTimer()
	for b.Loop() {
		ep.ServeHTTP(&w, &req)
		// if w.statusCode != http.StatusOK {
		// 	b.Fatalf("status code is not 200: %d", w.statusCode)
		// }
		// if string(w.written) != "1" {
		// 	b.Fatalf("written is not 1: %s", string(w.written))
		// }
	}
}

func BenchmarkEntrypoint(b *testing.B) {
	var ep Entrypoint
	var req = http.Request{
		Method: "GET",
		URL:    &url.URL{Path: "/", RawPath: "/"},
		Host:   "test.domain.tld",
	}
	ep.SetFindRouteDomains([]string{})

	r := &route.Route{
		Alias:  "test",
		Scheme: "http",
		Host:   "localhost",
		Port: route.Port{
			Proxy: 8080,
		},
		HealthCheck: &types.HealthCheckConfig{
			Disable: true,
		},
	}

	err := r.Validate()
	if err != nil {
		b.Fatal(err)
	}

	err = r.Start(task.RootTask("test", false))
	if err != nil {
		b.Fatal(err)
	}

	rev, ok := routes.HTTP.Get("test")
	if !ok {
		b.Fatal("route not found")
	}
	rev.(types.ReverseProxyRoute).ReverseProxy().Transport = noopTransport{}

	var w noopResponseWriter

	b.ResetTimer()
	for b.Loop() {
		ep.ServeHTTP(&w, &req)
		if w.statusCode != http.StatusOK {
			b.Fatalf("status code is not 200: %d", w.statusCode)
		}
	}
}

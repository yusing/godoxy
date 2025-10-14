package rules

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"testing"
)

func BenchmarkRules(b *testing.B) {
	var rules Rules
	err := parseRules(`
- name: admin-api
  on: |
    path glob(/api/admin/*)
    header Authorization
    method POST
  do: |
    set resp_header X-Access-Level "admin"
    set resp_header X-API-Version "v1"
- name: user-api
  on: |
    path glob(/api/users/*) & method GET
  do: |
    set resp_header X-Access-Level "user"
    set resp_header X-API-Version "v1"
- name: public-api
  on: |
    path glob(/api/public/*) & method GET
  do: |
    set resp_header X-Access-Level "public"
`, &rules)
	if err != nil {
		b.Fatal(err)
	}

	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	b.Run("BuildHandler", func(b *testing.B) {
		for b.Loop() {
			rules.BuildHandler(upstream)
		}
	})

	b.Run("RunHandler", func(b *testing.B) {
		var r = &http.Request{
			Body: io.NopCloser(bytes.NewReader([]byte(""))),
			URL:  &url.URL{Path: "/api/users/"},
		}
		var w noopResponseWriter
		handler := rules.BuildHandler(upstream)
		b.ResetTimer()
		for b.Loop() {
			handler.ServeHTTP(w, r)
		}
	})
}

type noopResponseWriter struct {
}

func (w noopResponseWriter) Header() http.Header {
	return http.Header{}
}

func (w noopResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (w noopResponseWriter) WriteHeader(int) {
}

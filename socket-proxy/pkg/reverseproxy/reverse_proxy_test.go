package reverseproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestReverseProxyStripsForwardedFromOutboundRequest(t *testing.T) {
	var gotForwarded string
	proxy := &ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "backend.local"
		},
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotForwarded = req.Header.Get("Forwarded")
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Header:     make(http.Header),
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "http://proxy.local/_ping", nil)
	req.Header.Set("Forwarded", "for=203.0.113.10;proto=https")
	rec := httptest.NewRecorder()

	proxy.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("proxy status = %d, want %d", res.StatusCode, http.StatusNoContent)
	}
	if gotForwarded != "" {
		t.Fatalf("outbound Forwarded header = %q, want removed", gotForwarded)
	}
}

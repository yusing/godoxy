package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	expect "github.com/yusing/goutils/testing"
)

type testPriority struct {
	Value int `json:"value"`
}

var test = NewMiddleware[testPriority]()
var responseRewrite = NewMiddleware[testResponseRewrite]()

func (t testPriority) before(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Add("Test-Value", strconv.Itoa(t.Value))
	return true
}

type testResponseRewrite struct {
	StatusCode int    `json:"status_code"`
	HeaderKey  string `json:"header_key"`
	HeaderVal  string `json:"header_val"`
	Body       string `json:"body"`
}

type closeSensitiveBody struct {
	data   []byte
	offset int
	closed bool
}

func (b *closeSensitiveBody) Read(p []byte) (int, error) {
	if b.closed {
		return 0, errors.New("http: read on closed response body")
	}
	if b.offset >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.offset:])
	b.offset += n
	return n, nil
}

func (b *closeSensitiveBody) Close() error {
	b.closed = true
	return nil
}

func (t testResponseRewrite) modifyResponse(resp *http.Response) error {
	resp.StatusCode = t.StatusCode
	resp.Header.Set(t.HeaderKey, t.HeaderVal)
	resp.Body = io.NopCloser(strings.NewReader(t.Body))
	return nil
}

func TestMiddlewarePriority(t *testing.T) {
	priorities := []int{4, 7, 9, 0}
	chain := make([]*Middleware, len(priorities))
	for i, p := range priorities {
		mid, err := test.New(OptionsRaw{
			"priority": p,
			"value":    i,
		})
		expect.NoError(t, err)
		chain[i] = mid
	}
	res, err := newMiddlewaresTest(chain, nil)
	expect.NoError(t, err)
	expect.Equal(t, strings.Join(res.ResponseHeaders["Test-Value"], ","), "3,0,1,2")
}

func TestMiddlewareResponseRewriteGate(t *testing.T) {
	opts := OptionsRaw{
		"status_code": 418,
		"header_key":  "X-Rewrite",
		"header_val":  "1",
		"body":        "rewritten-body",
	}

	tests := []struct {
		name        string
		respHeaders http.Header
		respBody    []byte
		expectBody  string
	}{
		{
			name: "allow_body_rewrite_for_html",
			respHeaders: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
			respBody:   []byte("<html><body>original</body></html>"),
			expectBody: "rewritten-body",
		},
		{
			name: "allow_body_rewrite_for_json",
			respHeaders: http.Header{
				"Content-Type": []string{"application/json"},
			},
			respBody:   []byte(`{"message":"original"}`),
			expectBody: "rewritten-body",
		},
		{
			name: "allow_body_rewrite_for_yaml",
			respHeaders: http.Header{
				"Content-Type": []string{"application/yaml"},
			},
			respBody:   []byte("k: v"),
			expectBody: "rewritten-body",
		},
		{
			name: "block_body_rewrite_for_binary_content",
			respHeaders: http.Header{
				"Content-Type": []string{"application/octet-stream"},
			},
			respBody:   []byte("binary"),
			expectBody: "binary",
		},
		{
			name: "block_body_rewrite_for_transfer_encoded_html",
			respHeaders: http.Header{
				"Content-Type":      []string{"text/html"},
				"Transfer-Encoding": []string{"chunked"},
			},
			respBody:   []byte("<html><body>original</body></html>"),
			expectBody: "<html><body>original</body></html>",
		},
		{
			name: "block_body_rewrite_for_content_encoded_html",
			respHeaders: http.Header{
				"Content-Type":     []string{"text/html"},
				"Content-Encoding": []string{"gzip"},
			},
			respBody:   []byte("<html><body>original</body></html>"),
			expectBody: "<html><body>original</body></html>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := newMiddlewareTest(responseRewrite, &testArgs{
				middlewareOpt: opts,
				respHeaders:   tc.respHeaders,
				respBody:      tc.respBody,
				respStatus:    http.StatusOK,
			})
			expect.NoError(t, err)
			expect.Equal(t, result.ResponseStatus, http.StatusTeapot)
			expect.Equal(t, result.ResponseHeaders.Get("X-Rewrite"), "1")
			expect.Equal(t, string(result.Data), tc.expectBody)
		})
	}
}

func TestMiddlewareResponseRewriteGateServeHTTP(t *testing.T) {
	opts := OptionsRaw{
		"status_code": 418,
		"header_key":  "X-Rewrite",
		"header_val":  "1",
		"body":        "rewritten-body",
	}

	tests := []struct {
		name             string
		respHeaders      http.Header
		respBody         string
		expectStatusCode int
		expectHeader     string
		expectBody       string
	}{
		{
			name: "allow_body_rewrite_for_html",
			respHeaders: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
			respBody:         "<html><body>original</body></html>",
			expectStatusCode: http.StatusTeapot,
			expectHeader:     "1",
			expectBody:       "rewritten-body",
		},
		{
			name: "block_body_rewrite_for_binary_content",
			respHeaders: http.Header{
				"Content-Type": []string{"application/octet-stream"},
			},
			respBody:         "binary",
			expectStatusCode: http.StatusOK,
			expectHeader:     "",
			expectBody:       "binary",
		},
		{
			name: "block_body_rewrite_for_transfer_encoded_html",
			respHeaders: http.Header{
				"Content-Type":      []string{"text/html"},
				"Transfer-Encoding": []string{"chunked"},
			},
			respBody:         "<html><body>original</body></html>",
			expectStatusCode: http.StatusOK,
			expectHeader:     "",
			expectBody:       "<html><body>original</body></html>",
		},
		{
			name: "block_body_rewrite_for_content_encoded_html",
			respHeaders: http.Header{
				"Content-Type":     []string{"text/html"},
				"Content-Encoding": []string{"gzip"},
			},
			respBody:         "<html><body>original</body></html>",
			expectStatusCode: http.StatusOK,
			expectHeader:     "",
			expectBody:       "<html><body>original</body></html>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mid, err := responseRewrite.New(opts)
			expect.NoError(t, err)

			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
			rw := httptest.NewRecorder()

			next := func(w http.ResponseWriter, _ *http.Request) {
				for key, values := range tc.respHeaders {
					for _, value := range values {
						w.Header().Add(key, value)
					}
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tc.respBody))
			}

			mid.ServeHTTP(next, rw, req)

			resp := rw.Result()
			defer resp.Body.Close()
			data, readErr := io.ReadAll(resp.Body)
			expect.NoError(t, readErr)

			expect.Equal(t, resp.StatusCode, tc.expectStatusCode)
			expect.Equal(t, resp.Header.Get("X-Rewrite"), tc.expectHeader)
			expect.Equal(t, string(data), tc.expectBody)
		})
	}
}

func TestMiddlewareResponseRewriteGateSkipsBodyRewriterWhenRewriteBlocked(t *testing.T) {
	originalBody := &closeSensitiveBody{
		data: []byte("<html><body>original</body></html>"),
	}
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":      []string{"text/html; charset=utf-8"},
			"Transfer-Encoding": []string{"chunked"},
		},
		Body:             originalBody,
		ContentLength:    -1,
		TransferEncoding: []string{"chunked"},
		Request:          req,
	}

	themedMid, err := Themed.New(OptionsRaw{
		"theme": DarkTheme,
	})
	expect.NoError(t, err)

	respMod, ok := themedMid.impl.(ResponseModifier)
	expect.True(t, ok)
	expect.NoError(t, modifyResponseWithBodyRewriteGate(respMod, resp))

	data, err := io.ReadAll(resp.Body)
	expect.NoError(t, err)
	expect.Equal(t, string(data), "<html><body>original</body></html>")
}

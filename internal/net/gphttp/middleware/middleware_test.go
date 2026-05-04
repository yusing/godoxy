package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	ioutils "github.com/yusing/goutils/io"
	expect "github.com/yusing/goutils/testing"
)

type testPriority struct {
	Value int `json:"value"`
}

var test = NewMiddleware[testPriority]()
var responseHeaderRewrite = NewMiddleware[testHeaderRewrite]()
var responseBodyRewrite = NewMiddleware[testBodyRewrite]()

func (t testPriority) before(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Add("Test-Value", strconv.Itoa(t.Value))
	return true
}

type testHeaderRewrite struct {
	StatusCode int    `json:"status_code"`
	HeaderKey  string `json:"header_key"`
	HeaderVal  string `json:"header_val"`
}

func (t testHeaderRewrite) modifyResponse(resp *http.Response) error {
	resp.StatusCode = t.StatusCode
	resp.Header.Set(t.HeaderKey, t.HeaderVal)
	return nil
}

type testBodyRewrite struct {
	Body string `json:"body"`
}

func (t testBodyRewrite) modifyResponse(resp *http.Response) error {
	resp.Body = io.NopCloser(strings.NewReader(t.Body))
	return nil
}

func (testBodyRewrite) isBodyResponseModifier() {}

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
	headerOpts := OptionsRaw{
		"status_code": 418,
		"header_key":  "X-Rewrite",
		"header_val":  "1",
	}
	bodyOpts := OptionsRaw{
		"body": "rewritten-body",
	}
	headerMid, err := responseHeaderRewrite.New(headerOpts)
	expect.NoError(t, err)
	bodyMid, err := responseBodyRewrite.New(bodyOpts)
	expect.NoError(t, err)

	tests := []struct {
		name         string
		respHeaders  http.Header
		respBody     []byte
		expectStatus int
		expectHeader string
		expectBody   string
	}{
		{
			name: "allow_body_rewrite_for_html",
			respHeaders: http.Header{
				"Content-Type": []string{"text/html; charset=utf-8"},
			},
			respBody:     []byte("<html><body>original</body></html>"),
			expectStatus: http.StatusTeapot,
			expectHeader: "1",
			expectBody:   "rewritten-body",
		},
		{
			name: "allow_body_rewrite_for_json",
			respHeaders: http.Header{
				"Content-Type": []string{"application/json"},
			},
			respBody:     []byte(`{"message":"original"}`),
			expectStatus: http.StatusTeapot,
			expectHeader: "1",
			expectBody:   "rewritten-body",
		},
		{
			name: "allow_body_rewrite_for_yaml",
			respHeaders: http.Header{
				"Content-Type": []string{"application/yaml"},
			},
			respBody:     []byte("k: v"),
			expectStatus: http.StatusTeapot,
			expectHeader: "1",
			expectBody:   "rewritten-body",
		},
		{
			name: "block_body_rewrite_for_binary_content",
			respHeaders: http.Header{
				"Content-Type": []string{"application/octet-stream"},
			},
			respBody:     []byte("binary"),
			expectStatus: http.StatusTeapot,
			expectHeader: "1",
			expectBody:   "binary",
		},
		{
			name: "allow_body_rewrite_for_transfer_encoded_html",
			respHeaders: http.Header{
				"Content-Type":      []string{"text/html"},
				"Transfer-Encoding": []string{"chunked"},
			},
			respBody:     []byte("<html><body>original</body></html>"),
			expectStatus: http.StatusTeapot,
			expectHeader: "1",
			expectBody:   "rewritten-body",
		},
		{
			name: "block_body_rewrite_for_non_chunked_transfer_encoded_html",
			respHeaders: http.Header{
				"Content-Type":      []string{"text/html"},
				"Transfer-Encoding": []string{"gzip"},
			},
			respBody:     []byte("<html><body>original</body></html>"),
			expectStatus: http.StatusTeapot,
			expectHeader: "1",
			expectBody:   "<html><body>original</body></html>",
		},
		{
			name: "block_body_rewrite_for_content_encoded_html",
			respHeaders: http.Header{
				"Content-Type":     []string{"text/html"},
				"Content-Encoding": []string{"gzip"},
			},
			respBody:     []byte("<html><body>original</body></html>"),
			expectStatus: http.StatusTeapot,
			expectHeader: "1",
			expectBody:   "<html><body>original</body></html>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := newMiddlewaresTest([]*Middleware{headerMid, bodyMid}, &testArgs{
				respHeaders: tc.respHeaders,
				respBody:    tc.respBody,
				respStatus:  http.StatusOK,
			})
			expect.NoError(t, err)
			expect.Equal(t, result.ResponseStatus, tc.expectStatus)
			expect.Equal(t, result.ResponseHeaders.Get("X-Rewrite"), tc.expectHeader)
			expect.Equal(t, string(result.Data), tc.expectBody)
		})
	}
}

func TestMiddlewareResponseRewriteGateServeHTTP(t *testing.T) {
	headerOpts := OptionsRaw{
		"status_code": 418,
		"header_key":  "X-Rewrite",
		"header_val":  "1",
	}
	bodyOpts := OptionsRaw{
		"body": "rewritten-body",
	}
	headerMid, err := responseHeaderRewrite.New(headerOpts)
	expect.NoError(t, err)
	bodyMid, err := responseBodyRewrite.New(bodyOpts)
	expect.NoError(t, err)
	mid := NewMiddlewareChain("test", []*Middleware{headerMid, bodyMid})

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
			name: "allow_body_rewrite_for_transfer_encoded_html",
			respHeaders: http.Header{
				"Content-Type":      []string{"text/html"},
				"Transfer-Encoding": []string{"chunked"},
			},
			respBody:         "<html><body>original</body></html>",
			expectStatusCode: http.StatusTeapot,
			expectHeader:     "1",
			expectBody:       "rewritten-body",
		},
		{
			name: "block_body_rewrite_for_non_chunked_transfer_encoded_html",
			respHeaders: http.Header{
				"Content-Type":      []string{"text/html"},
				"Transfer-Encoding": []string{"gzip"},
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

func TestMiddlewareHeaderRewriteDoesNotBufferLargeBody(t *testing.T) {
	headerMid, err := responseHeaderRewrite.New(OptionsRaw{
		"status_code": http.StatusAccepted,
		"header_key":  "X-Rewrite",
		"header_val":  "1",
	})
	expect.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	rw := httptest.NewRecorder()

	next := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", strconv.Itoa(64*1024*1024))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("video"))
	}

	headerMid.ServeHTTP(next, rw, req)

	resp := rw.Result()
	defer resp.Body.Close()
	data, readErr := io.ReadAll(resp.Body)
	expect.NoError(t, readErr)

	expect.Equal(t, resp.StatusCode, http.StatusAccepted)
	expect.Equal(t, resp.Header.Get("X-Rewrite"), "1")
	expect.Equal(t, string(data), "video")
}

func TestThemedRewritesChunkedHTML(t *testing.T) {
	result, err := newMiddlewareTest(Themed, &testArgs{
		middlewareOpt: OptionsRaw{
			"css": "https://example.com/theme.css",
		},
		respHeaders: http.Header{
			"Content-Type":      []string{"text/html; charset=utf-8"},
			"Transfer-Encoding": []string{"chunked"},
		},
		respBody: []byte("<html><body>original</body></html>"),
	})
	expect.NoError(t, err)
	expect.Equal(t, string(result.Data), `<html><head></head><body>original<link rel="stylesheet" href="https://example.com/theme.css"/></body></html>`)
}

func TestThemedSkipsOversizedChunkedHTML(t *testing.T) {
	originalBody := "<html><body>" + strings.Repeat("a", maxModifiableBody) + "</body></html>"

	result, err := newMiddlewareTest(Themed, &testArgs{
		middlewareOpt: OptionsRaw{
			"css": "https://example.com/theme.css",
		},
		respHeaders: http.Header{
			"Content-Type":      []string{"text/html; charset=utf-8"},
			"Transfer-Encoding": []string{"chunked"},
		},
		respBody: []byte(originalBody),
	})
	expect.NoError(t, err)
	expect.Equal(t, string(result.Data), originalBody)
}

func TestMiddlewareResponseRewriteGateServeHTTPIgnoresUnsupportedBufferedFlush(t *testing.T) {
	bodyMid, err := responseBodyRewrite.New(OptionsRaw{"body": "rewritten-body"})
	expect.NoError(t, err)
	mid := NewMiddlewareChain("test", []*Middleware{bodyMid})

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	rw := httptest.NewRecorder()

	next := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		err := ioutils.CopyCloseWithContext(req.Context(), w, strings.NewReader("<html><body>original</body></html>"), -1)
		expect.NoError(t, err)
	}

	mid.ServeHTTP(next, rw, req)

	resp := rw.Result()
	defer resp.Body.Close()
	data, readErr := io.ReadAll(resp.Body)
	expect.NoError(t, readErr)
	expect.Equal(t, resp.StatusCode, http.StatusOK)
	expect.Equal(t, string(data), "rewritten-body")
}

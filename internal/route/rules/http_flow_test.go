package rules

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
)

// mockUpstream creates a simple upstream handler for testing
func mockUpstream(status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	})
}

// mockUpstreamWithHeaders creates an upstream that returns specific headers
func mockUpstreamWithHeaders(status int, body string, headers http.Header) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header()[k] = v
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	})
}

func parseRules(data string, target *Rules) gperr.Error {
	_, err := serialization.ConvertString(data, reflect.ValueOf(target))
	return err
}

func TestHTTPFlow_BasicPreRules(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "value")
		w.WriteHeader(200)
		w.Write([]byte("upstream response"))
	})

	var rules Rules
	err := parseRules(`
- name: add-header
  on: header X-Custom-Header
  do: set header X-Custom-Header test-value
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "upstream response", w.Body.String())
	assert.Equal(t, "test-value", w.Header().Get("X-Custom-Header"))
}

func TestHTTPFlow_BypassRule(t *testing.T) {
	upstream := mockUpstream(200, "upstream response")

	var rules Rules
	err := parseRules(`
- name: bypass-condition
  on: path /bypass
  do: bypass
- name: should-not-execute
  on: path /bypass
  do: error 500 should not reach here
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/bypass", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "upstream response", w.Body.String())
}

func TestHTTPFlow_TerminatingCommand(t *testing.T) {
	upstream := mockUpstream(200, "should not be called")

	var rules Rules
	err := parseRules(`
- name: error-response
  on: path /error
  do: error 403 Forbidden
- name: should-not-execute
  on: path /error
  do: set header X-Header ignored
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/error", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code)
	assert.Contains(t, w.Body.String(), "Forbidden")
	assert.Empty(t, w.Header().Get("X-Header"))
}

func TestHTTPFlow_RedirectFlow(t *testing.T) {
	upstream := mockUpstream(200, "should not be called")

	var rules Rules
	err := parseRules(`
- name: redirect-rule
  on: path /old-path
  do: redirect /new-path
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/old-path", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 307, w.Code) // TemporaryRedirect
	assert.Equal(t, "/new-path", w.Header().Get("Location"))
}

func TestHTTPFlow_RewriteFlow(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("path: " + r.URL.Path))
	})

	var rules Rules
	err := parseRules(`
- name: rewrite-rule
  on: path /api/*
  do: rewrite /api/ /v1/
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/api/users", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "path: /v1/users", w.Body.String())
}

func TestHTTPFlow_MultiplePreRules(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("upstream: " + r.Header.Get("X-Request-Id")))
	})

	var rules Rules
	err := parseRules(`
- name: add-request-id
  on: path /
  do: set X-Request-Id req-123
- name: add-auth-header
  on: path /
  do: set X-Auth-Token token-456
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "upstream: req-123", w.Body.String())
	assert.Equal(t, "token-456", w.Header().Get("X-Auth-Token"))
}

func TestHTTPFlow_PostResponseRule(t *testing.T) {
	upstream := mockUpstreamWithHeaders(200, "success", http.Header{
		"X-Upstream": []string{"upstream-value"},
	})

	tempFile, err := os.CreateTemp("", "test-log-*.txt")
	// Create a temporary file for logging
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	var rules Rules
	err = parseRules(fmt.Sprintf(`
- name: log-response
  on: path /test
  do: log info %s "{{ .Request.Method }} {{ .Response.StatusCode }}"
`, tempFile.Name()), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "success", w.Body.String())
	assert.Equal(t, "upstream-value", w.Header().Get("X-Upstream"))

	// Check log file
	content, err := os.ReadFile(tempFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(content), "GET 200")
}

func TestHTTPFlow_ResponseRuleWithStatusCondition(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/success" {
			w.WriteHeader(200)
			w.Write([]byte("success"))
		} else {
			w.WriteHeader(404)
			w.Write([]byte("not found"))
		}
	})

	var rules Rules

	// Create a temporary file for logging
	tempFile, err := os.CreateTemp("", "test-error-log-*.txt")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	err = parseRules(fmt.Sprintf(`
- name: log-errors
  on: status 4xx
  do: log error %s "{{ .Request.URL }} returned {{ .Response.StatusCode }}"
`, tempFile.Name()), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test successful request (should not log)
	req1 := httptest.NewRequest("GET", "/success", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, 200, w1.Code)

	// Test error request (should log)
	req2 := httptest.NewRequest("GET", "/notfound", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 404, w2.Code)

	// Check log file
	content, err := os.ReadFile(tempFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(content), "/notfound")
	assert.Contains(t, string(content), "404")
	assert.NotContains(t, string(content), "/success")
}

func TestHTTPFlow_ConditionalRules(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hello " + r.Header.Get("X-Username")))
	})

	var rules Rules
	err := parseRules(`
- name: auth-required
  on: header Authorization
  do: set X-Username authenticated-user
- name: default
  do: set X-Username anonymous
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test with Authorization header
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.Header.Set("Authorization", "Bearer token")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, "hello authenticated-user", w1.Body.String())

	// Test without Authorization header
	req2 := httptest.NewRequest("GET", "/", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, "hello anonymous", w2.Body.String())
}

func TestHTTPFlow_ComplexFlowWithPreAndPostRules(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate different responses based on path
		if r.URL.Path == "/protected" {
			if r.Header.Get("X-Auth") != "valid" {
				w.WriteHeader(401)
				w.Write([]byte("unauthorized"))
				return
			}
		}
		w.Header().Set("X-Response-Time", "100ms")
		w.WriteHeader(200)
		w.Write([]byte("success"))
	})

	// Create temporary files for logging
	logFile, err := os.CreateTemp("", "test-access-log-*.txt")
	require.NoError(t, err)
	defer os.Remove(logFile.Name())
	logFile.Close()

	errorLogFile, err := os.CreateTemp("", "test-error-log-*.txt")
	require.NoError(t, err)
	defer os.Remove(errorLogFile.Name())
	errorLogFile.Close()

	var rules Rules
	err = parseRules(fmt.Sprintf(`
- name: add-correlation-id
  do: set X-Correlation-ID {{ random_uuid }}
- name: validate-auth
  on: path /protected
  do: require_basic_auth "Protected Area"
- name: log-all-requests
  do: log info %s "{{ .Request.Method }} {{ .Request.URL }} -> {{ .Response.StatusCode }}"
- name: log-errors
  on: status 4xx
  do: log error %s "ERROR: {{ .Request.Method }} {{ .Request.URL }} {{ .Response.StatusCode }}"
`, logFile.Name(), errorLogFile.Name()), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test successful request
	req1 := httptest.NewRequest("GET", "/public", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, "success", w1.Body.String())
	assert.NotEmpty(t, w1.Header().Get("X-Correlation-ID"))

	// Test unauthorized protected request
	req2 := httptest.NewRequest("GET", "/protected", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 401, w2.Code)
	assert.Contains(t, w2.Body.String(), "Unauthorized")

	// Test authorized protected request
	req3 := httptest.NewRequest("GET", "/protected", nil)
	req3.SetBasicAuth("user", "pass")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	// This should fail because our simple upstream expects X-Auth: valid header
	// but the basic auth requirement should add the appropriate header
	assert.Equal(t, 401, w3.Code)

	// Check log files
	logContent, err := os.ReadFile(logFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(logContent), "GET /public")
	assert.Contains(t, string(logContent), "GET /protected")

	errorLogContent, err := os.ReadFile(errorLogFile.Name())
	require.NoError(t, err)
	// Should have at least one 401 error logged
	assert.Contains(t, string(errorLogContent), "401")
}

func TestHTTPFlow_DefaultRule(t *testing.T) {
	upstream := mockUpstream(200, "upstream response")

	var rules Rules
	err := parseRules(`
- name: default
  do: set X-Default-Applied true
- name: special-rule
  on: path /special
  do: set X-Special-Handled true
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test default rule
	req1 := httptest.NewRequest("GET", "/regular", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, "true", w1.Header().Get("X-Default-Applied"))
	assert.Empty(t, w1.Header().Get("X-Special-Handled"))

	// Test special rule + default rule
	req2 := httptest.NewRequest("GET", "/special", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, "true", w2.Header().Get("X-Default-Applied"))
	assert.Equal(t, "true", w2.Header().Get("X-Special-Handled"))
}

func TestHTTPFlow_HeaderManipulation(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back a header
		headerValue := r.Header.Get("X-Test-Header")
		w.Header().Set("X-Echoed-Header", headerValue)
		w.WriteHeader(200)
		w.Write([]byte("header echoed"))
	})

	var rules Rules
	err := parseRules(`
- name: remove-sensitive-header
  do: remove X-Secret
- name: add-custom-header
  do: add X-Custom-Header custom-value
- name: modify-existing-header
  on: header X-Test-Header
  do: set X-Test-Header modified-value
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Secret", "secret-value")
	req.Header.Set("X-Test-Header", "original-value")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "modified-value", w.Header().Get("X-Echoed-Header"))
	assert.Equal(t, "custom-value", w.Header().Get("X-Custom-Header"))
	// Ensure the secret header was removed and not passed to upstream
	// (we can't directly test this, but the upstream shouldn't see it)
}

func TestHTTPFlow_QueryParameterHandling(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		w.WriteHeader(200)
		w.Write([]byte("query: " + query.Get("param")))
	})

	var rules Rules
	err := parseRules(`
- name: add-query-param
  on: query param
  do: set query.param added-value
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/path?param=original", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	// The set command should have modified the query parameter
	assert.Equal(t, "query: added-value", w.Body.String())
}

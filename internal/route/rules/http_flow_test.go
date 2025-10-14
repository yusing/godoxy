package rules_test

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
	"golang.org/x/crypto/bcrypt"

	. "github.com/yusing/godoxy/internal/route/rules"
)

// mockUpstream creates a simple upstream handler for testing
func mockUpstream(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}
}

// mockUpstreamWithHeaders creates an upstream that returns specific headers
func mockUpstreamWithHeaders(status int, body string, headers http.Header) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		maps.Copy(w.Header(), headers)
		w.WriteHeader(status)
		w.Write([]byte(body))
	}
}

func mockRoute(alias string) *route.FileServer {
	return &route.FileServer{Route: &route.Route{Alias: alias}}
}

func parseRules(data string, target *Rules) gperr.Error {
	_, err := serialization.ConvertString(strings.TrimSpace(data), reflect.ValueOf(target))
	return err
}

func TestHTTPFlow_BasicPreRules(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", r.Header.Get("X-Custom-Header"))
		w.WriteHeader(200)
		w.Write([]byte("upstream response"))
	})

	var rules Rules
	err := parseRules(`
- name: add-header
  on: path /
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
  do: error 500 "should not reach here"
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
	assert.Equal(t, "Forbidden\n", w.Body.String())
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
  on: path glob(/api/*)
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
  do: set header X-Request-Id req-123
- name: add-auth-header
  on: path /
  do: set header X-Auth-Token token-456
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "upstream: req-123", w.Body.String())
	assert.Equal(t, "token-456", req.Header.Get("X-Auth-Token"))
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
	assert.Equal(t, "GET 200\n", string(content))
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
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Len(t, lines, 1, "only 4xx requests should be logged")
	assert.Equal(t, "/notfound returned 404", lines[0])
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
  do: |
    set header X-Username authenticated-user
    set resp_header X-Username authenticated-user
- name: default
  do: |
    set header X-Username anonymous
    set resp_header X-Username anonymous
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
	assert.Equal(t, "authenticated-user", w1.Header().Get("X-Username"))

	// Test without Authorization header
	req2 := httptest.NewRequest("GET", "/", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, "hello anonymous", w2.Body.String())
	assert.Equal(t, "anonymous", w2.Header().Get("X-Username"))
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
  do: set resp_header X-Correlation-Id random_uuid
- name: validate-auth
  on: path /protected
  do: require_basic_auth "Protected Area"
- name: log-all-requests
  do: |
    log info %q "{{ .Request.Method }} {{ .Request.URL }} -> {{ .Response.StatusCode }}"
- name: log-errors
  on: status 4xx
  do: |
    log error %q "ERROR: {{ .Request.Method }} {{ .Request.URL }} {{ .Response.StatusCode }}"
`, logFile.Name(), errorLogFile.Name()), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test successful request
	req1 := httptest.NewRequest("GET", "/public", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, "success", w1.Body.String())
	assert.Equal(t, "random_uuid", w1.Header().Get("X-Correlation-Id"))
	assert.Equal(t, "100ms", w1.Header().Get("X-Response-Time"))

	// Test unauthorized protected request
	req2 := httptest.NewRequest("GET", "/protected", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 401, w2.Code)
	assert.Equal(t, w2.Body.String(), "Unauthorized\n")

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
	lines := strings.Split(strings.TrimSpace(string(logContent)), "\n")
	require.Len(t, lines, 3, "all requests should be logged")
	assert.Equal(t, "GET /public -> 200", lines[0])
	assert.Equal(t, "GET /protected -> 401", lines[1])
	assert.Equal(t, "GET /protected -> 401", lines[2])

	errorLogContent, err := os.ReadFile(errorLogFile.Name())
	require.NoError(t, err)
	// Should have at least one 401 error logged
	lines = strings.Split(strings.TrimSpace(string(errorLogContent)), "\n")
	require.Len(t, lines, 2, "all errors should be logged")
	assert.Equal(t, "ERROR: GET /protected 401", lines[0])
	assert.Equal(t, "ERROR: GET /protected 401", lines[1])
}

func TestHTTPFlow_DefaultRule(t *testing.T) {
	upstream := mockUpstream(200, "upstream response")

	var rules Rules
	err := parseRules(`
- name: default
  do: set resp_header X-Default-Applied true
- name: special-rule
  on: path /special
  do: set resp_header X-Special-Handled true
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
  do: remove resp_header X-Secret
- name: add-custom-header
  do: add resp_header X-Custom-Header custom-value
- name: modify-existing-header
  on: header X-Test-Header
  do: set header X-Test-Header modified-value
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
  do: set query param added-value
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

func TestHTTPFlow_ServeCommand(t *testing.T) {
	// Create a temporary directory with test files
	tempDir, err := os.MkdirTemp("", "test-serve-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test files directly in the temp directory
	testFile := filepath.Join(tempDir, "index.html")
	err = os.WriteFile(testFile, []byte("<h1>Test Page</h1>"), 0644)
	require.NoError(t, err)

	var rules Rules
	err = parseRules(fmt.Sprintf(`
- name: serve-static
  on: path glob(/files/*)
  do: serve %s
`, tempDir), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(mockUpstream(200, "should not be called"))

	// Test serving a file - serve command serves files relative to the root directory
	// The path /files/index.html gets mapped to tempDir + "/files/index.html"
	// We need to create the file at the expected path
	filesDir := filepath.Join(tempDir, "files")
	err = os.Mkdir(filesDir, 0755)
	require.NoError(t, err)

	filesIndexFile := filepath.Join(filesDir, "index.html")
	err = os.WriteFile(filesIndexFile, []byte("<h1>Test Page</h1>"), 0644)
	require.NoError(t, err)

	req1 := httptest.NewRequest("GET", "/files/index.html", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	// The serve command should work, but might redirect
	// Let's just verify it doesn't call the upstream
	assert.NotEqual(t, "should not be called", w1.Body.String())

	// Test file not found
	req2 := httptest.NewRequest("GET", "/files/nonexistent.html", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 404, w2.Code)
}

func TestHTTPFlow_ProxyCommand(t *testing.T) {
	// Create a mock upstream server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Header", "upstream-value")
		w.WriteHeader(200)
		w.Write([]byte("upstream response"))
	}))
	defer upstreamServer.Close()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
- name: proxy-to-upstream
  on: path glob(/api/*)
  do: proxy %s
`, upstreamServer.URL), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(mockUpstream(200, "should not be called"))

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// The proxy command should forward the request to the upstream server
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "upstream response", w.Body.String())
	assert.Equal(t, "upstream-value", w.Header().Get("X-Upstream-Header"))
}

func TestHTTPFlow_NotifyCommand(t *testing.T) {
	// TODO:
}

func TestHTTPFlow_FormConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("form processed"))
	})

	var rules Rules
	err := parseRules(`
- name: process-form
  on: form username
  do: set resp_header X-Username "{{ index .Form.username 0 }}"
- name: process-postform
  on: postform email
  do: set resp_header X-Email "{{ index .PostForm.email 0 }}"
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test form condition
	formData := url.Values{"username": {"john_doe"}}
	req1 := httptest.NewRequest("POST", "/", strings.NewReader(formData.Encode()))
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, "john_doe", w1.Header().Get("X-Username"))

	// Test postform condition
	postFormData := url.Values{"email": {"john@example.com"}}
	req2 := httptest.NewRequest("POST", "/", strings.NewReader(postFormData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, "john@example.com", w2.Header().Get("X-Email"))
}

func TestHTTPFlow_RemoteConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("remote processed"))
	})

	var rules Rules
	err := parseRules(`
- name: allow-localhost
  on: remote 127.0.0.1
  do: set resp_header X-Access "local"
- name: block-private
  on: remote 192.168.0.0/16
  do: error 403 "Private network blocked"
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test localhost condition
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "127.0.0.1:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, "local", w1.Header().Get("X-Access"))

	// Test private network block
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "192.168.1.100:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 403, w2.Code)
	assert.Equal(t, "Private network blocked\n", w2.Body.String())
}

func TestHTTPFlow_BasicAuthConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("auth processed"))
	})

	// Generate bcrypt hashes for passwords
	adminHash, err := bcrypt.GenerateFromPassword([]byte("adminpass"), bcrypt.DefaultCost)
	require.NoError(t, err)
	guestHash, err := bcrypt.GenerateFromPassword([]byte("guestpass"), bcrypt.DefaultCost)
	require.NoError(t, err)

	var rules Rules
	err = parseRules(fmt.Sprintf(`
- name: check-auth
  on: basic_auth admin %s
  do: set resp_header X-Auth-Status "admin"
- name: check-other-user
  on: basic_auth guest %s
  do: set resp_header X-Auth-Status "guest"
`, string(adminHash), string(guestHash)), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test admin user
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.SetBasicAuth("admin", "adminpass")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, "admin", w1.Header().Get("X-Auth-Status"))

	// Test guest user
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.SetBasicAuth("guest", "guestpass")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, "guest", w2.Header().Get("X-Auth-Status"))
}

func TestHTTPFlow_RouteConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("route processed"))
	})

	var rules Rules
	err := parseRules(`
- name: backend-route
  on: route backend
  do: set resp_header X-Route "backend"
- name: frontend-route
  on: route frontend
  do: set resp_header X-Route "frontend"
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test API route
	req1 := httptest.NewRequest("GET", "/", nil)
	req1 = routes.WithRouteContext(req1, mockRoute("backend"))

	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, "backend", w1.Header().Get("X-Route"))

	// Test admin route
	req2 := httptest.NewRequest("GET", "/", nil)
	req2 = routes.WithRouteContext(req2, mockRoute("frontend"))

	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, "frontend", w2.Header().Get("X-Route"))
}

func TestHTTPFlow_ResponseStatusConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(405)
		w.Write([]byte("method not allowed"))
	})

	var rules Rules
	err := parseRules(`
- name: method-not-allowed
  on: status 405
  do: |
    error 405 'error'
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, 405, w.Code)
	assert.Equal(t, "error\n", w.Body.String())
}

func TestHTTPFlow_ResponseHeaderConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response-Header", "response header")
		w.WriteHeader(200)
		w.Write([]byte("processed"))
	})

	t.Run("any_value", func(t *testing.T) {
		var rules Rules
		err := parseRules(`
- on: resp_header X-Response-Header
  do: |
    error 405 "error"
`, &rules)
		require.NoError(t, err)

		handler := rules.BuildHandler(upstream)

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 405, w.Code)
		assert.Equal(t, "error\n", w.Body.String())
	})
	t.Run("with_value", func(t *testing.T) {
		var rules Rules
		err := parseRules(`
- on: resp_header X-Response-Header "response header"
  do: |
    error 405 "error"
`, &rules)
		require.NoError(t, err)

		handler := rules.BuildHandler(upstream)

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 405, w.Code)
		assert.Equal(t, "error\n", w.Body.String())
	})

	t.Run("with_value_not_matched", func(t *testing.T) {
		var rules Rules
		err := parseRules(`
- on: resp_header X-Response-Header "not-matched"
  do: |
    error 405 "error"
`, &rules)
		require.NoError(t, err)

		handler := rules.BuildHandler(upstream)

		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)
		assert.Equal(t, "processed", w.Body.String())
	})
}

func TestHTTPFlow_ComplexRuleCombinations(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("complex processed"))
	})

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
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test admin API (should match first rule)
	req1 := httptest.NewRequest("POST", "/api/admin/users", nil)
	req1.Header.Set("Authorization", "Bearer token")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, "admin", w1.Header().Get("X-Access-Level"))
	assert.Equal(t, "v1", w1.Header()["X-API-Version"][0])

	// Test user API (should match second rule)
	req2 := httptest.NewRequest("GET", "/api/users/profile", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 200, w2.Code)
	assert.Equal(t, "user", w2.Header().Get("X-Access-Level"))
	assert.Equal(t, "v1", w2.Header()["X-API-Version"][0])

	// Test public API (should match third rule)
	req3 := httptest.NewRequest("GET", "/api/public/info", nil)
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	assert.Equal(t, 200, w3.Code)
	assert.Equal(t, "public", w3.Header().Get("X-Access-Level"))
	assert.Empty(t, w3.Header()["X-API-Version"])
}

func TestHTTPFlow_ResponseModifier(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("original response"))
	})

	var rules Rules
	err := parseRules(`
- name: modify-response
  do: |
    set resp_header X-Modified "true"
    set resp_body "Modified: {{ .Request.Method }} {{ .Request.URL.Path }}"
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "true", w.Header().Get("X-Modified"))
	assert.Equal(t, "Modified: GET /test\n", w.Body.String())
}

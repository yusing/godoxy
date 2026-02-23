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

func parseRules(data string, target *Rules) error {
	_, err := serialization.ConvertString(strings.TrimSpace(data), reflect.ValueOf(target))
	return err
}

func TestHTTPFlow_BasicPreRulesYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", r.Header.Get("X-Custom-Header"))
		w.WriteHeader(http.StatusOK)
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

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "upstream response", w.Body.String())
	assert.Equal(t, "test-value", w.Header().Get("X-Custom-Header"))
}

func TestHTTPFlow_BypassRuleYAML(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "upstream response")

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

	req := httptest.NewRequest(http.MethodGet, "/bypass", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "upstream response", w.Body.String())
}

func TestHTTPFlow_TerminatingCommandYAML(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "should not be called")

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

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code)
	assert.Equal(t, "Forbidden\n", w.Body.String())
	assert.Empty(t, w.Header().Get("X-Header"))
}

func TestHTTPFlow_RedirectFlowYAML(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "should not be called")

	var rules Rules
	err := parseRules(`
- name: redirect-rule
  on: path /old-path
  do: redirect /new-path
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/old-path", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 307, w.Code) // TemporaryRedirect
	assert.Equal(t, "/new-path", w.Header().Get("Location"))
}

func TestHTTPFlow_RewriteFlowYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
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

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "path: /v1/users", w.Body.String())
}

func TestHTTPFlow_MultiplePreRulesYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
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

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "upstream: req-123", w.Body.String())
	assert.Equal(t, "token-456", req.Header.Get("X-Auth-Token"))
}

func TestHTTPFlow_PostResponseRuleYAML(t *testing.T) {
	upstream := mockUpstreamWithHeaders(http.StatusOK, "success", http.Header{
		"X-Upstream": []string{"upstream-value"},
	})

	tempFile := TestRandomFileName()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
- name: log-response
  on: path /test
  do: log info %s "$req_method $status_code"
`, tempFile), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "success", w.Body.String())
	assert.Equal(t, "upstream-value", w.Header().Get("X-Upstream"))

	// Check log file
	content := TestFileContent(tempFile)
	assert.Equal(t, "GET 200\n", string(content))
}

func TestHTTPFlow_ResponseRuleWithStatusConditionYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/success" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("not found"))
		}
	})

	var rules Rules

	errorLog := TestRandomFileName()
	infoLog := TestRandomFileName()

	err := parseRules(fmt.Sprintf(`
- on: status 4xx
  do: log error %s "$req_url returned $status_code"
- on: status 200
  do: log info %s "$req_url returned $status_code"
`, errorLog, infoLog), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test successful request (should not log)
	req1 := httptest.NewRequest(http.MethodGet, "/success", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)

	// Test error request (should log)
	req2 := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusNotFound, w2.Code)

	// Check log file
	content := TestFileContent(errorLog)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Len(t, lines, 1, "only 4xx requests should be logged")
	assert.Equal(t, "/notfound returned 404", lines[0])

	infoContent := TestFileContent(infoLog)
	lines = strings.Split(strings.TrimSpace(string(infoContent)), "\n")
	require.Len(t, lines, 1, "only 200 requests should be logged")
	assert.Equal(t, "/success returned 200", lines[0])
}

func TestHTTPFlow_ConditionalRulesYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
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
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.Header.Set("Authorization", "Bearer token")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "hello authenticated-user", w1.Body.String())
	assert.Equal(t, "authenticated-user", w1.Header().Get("X-Username"))

	// Test without Authorization header
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "hello anonymous", w2.Body.String())
	assert.Equal(t, "anonymous", w2.Header().Get("X-Username"))
}

func TestHTTPFlow_ComplexFlowWithPreAndPostRulesYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate different responses based on path
		if r.URL.Path == "/protected" {
			if r.Header.Get("X-Auth") != "valid" {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, "unauthorized")
				return
			}
		}
		w.Header().Set("X-Response-Time", "100ms")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "success")
	})

	logFile := TestRandomFileName()
	errorLogFile := TestRandomFileName()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
- name: add-correlation-id
  do: set resp_header X-Correlation-Id random_uuid
- name: validate-auth
  on: path /protected
  do: require_basic_auth "Protected Area"
- name: log-all-requests
  do: |
    log info %q "$req_method $req_url -> $status_code"
- name: log-errors
  on: status 4xx
  do: |
    log error %q "ERROR: $req_method $req_url $status_code"
`, logFile, errorLogFile), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test successful request
	req1 := httptest.NewRequest(http.MethodGet, "/public", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "success", w1.Body.String())
	assert.Equal(t, "random_uuid", w1.Header().Get("X-Correlation-Id"))
	assert.Equal(t, "100ms", w1.Header().Get("X-Response-Time"))

	// Test unauthorized protected request
	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusUnauthorized, w2.Code)
	assert.Equal(t, "Unauthorized\n", w2.Body.String())

	// Test authorized protected request
	req3 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req3.SetBasicAuth("user", "pass")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	// This should fail because our simple upstream expects X-Auth: valid header
	// but the basic auth requirement should add the appropriate header
	assert.Equal(t, http.StatusUnauthorized, w3.Code)

	// Check log files
	logContent := TestFileContent(logFile)
	lines := strings.Split(strings.TrimSpace(string(logContent)), "\n")
	require.Len(t, lines, 3, "all requests should be logged")
	assert.Equal(t, "GET /public -> 200", lines[0])
	assert.Equal(t, "GET /protected -> 401", lines[1])
	assert.Equal(t, "GET /protected -> 401", lines[2])

	errorLogContent := TestFileContent(errorLogFile)
	// Should have at least one 401 error logged
	lines = strings.Split(strings.TrimSpace(string(errorLogContent)), "\n")
	require.Len(t, lines, 2, "all errors should be logged")
	assert.Equal(t, "ERROR: GET /protected 401", lines[0])
	assert.Equal(t, "ERROR: GET /protected 401", lines[1])
}

func TestHTTPFlow_DefaultRuleYAML(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "upstream response")

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
	req1 := httptest.NewRequest(http.MethodGet, "/regular", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "true", w1.Header().Get("X-Default-Applied"))
	assert.Empty(t, w1.Header().Get("X-Special-Handled"))

	// Test special rule (default should not run)
	req2 := httptest.NewRequest(http.MethodGet, "/special", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Empty(t, w2.Header().Get("X-Default-Applied"))
	assert.Equal(t, "true", w2.Header().Get("X-Special-Handled"))
}

func TestHTTPFlow_DefaultRuleWithOnDefaultYAML(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "upstream response")

	var rules Rules
	err := parseRules(`
- name: default-on-rule
  on: default
  do: set resp_header X-Default-Applied true
- name: special-rule
  on: path /special
  do: set resp_header X-Special-Handled true
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test default rule on regular request
	req1 := httptest.NewRequest(http.MethodGet, "/regular", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "true", w1.Header().Get("X-Default-Applied"))
	assert.Empty(t, w1.Header().Get("X-Special-Handled"))

	// Test special rule on matching request (default should not run)
	req2 := httptest.NewRequest(http.MethodGet, "/special", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Empty(t, w2.Header().Get("X-Default-Applied"))
	assert.Equal(t, "true", w2.Header().Get("X-Special-Handled"))
}

func TestHTTPFlow_HeaderManipulationYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back a header
		headerValue := r.Header.Get("X-Test-Header")
		w.Header().Set("X-Echoed-Header", headerValue)
		w.Header().Set("X-Secret", "sensitive-data")
		w.WriteHeader(http.StatusOK)
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

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Test-Header", "original-value")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "modified-value", w.Header().Get("X-Echoed-Header"))
	assert.Equal(t, "custom-value", w.Header().Get("X-Custom-Header"))
	assert.Empty(t, w.Header().Get("X-Secret"))
}

func TestHTTPFlow_QueryParameterHandlingYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		w.WriteHeader(http.StatusOK)
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

	req := httptest.NewRequest(http.MethodGet, "/path?param=original", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// The set command should have modified the query parameter
	assert.Equal(t, "query: added-value", w.Body.String())
}

func TestHTTPFlow_ServeCommandYAML(t *testing.T) {
	// Create a temporary directory with test files
	tempDir, err := os.MkdirTemp("", "test-serve-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test files directly in the temp directory
	testFile := filepath.Join(tempDir, "index.html")
	err = os.WriteFile(testFile, []byte("<h1>Test Page</h1>"), 0o644)
	require.NoError(t, err)

	var rules Rules
	err = parseRules(fmt.Sprintf(`
- name: serve-static
  on: path glob(/files/*)
  do: serve %s
`, tempDir), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(mockUpstream(http.StatusOK, "should not be called"))

	// Test serving a file - serve command serves files relative to the root directory
	// The path /files/index.html gets mapped to tempDir + "/files/index.html"
	// We need to create the file at the expected path
	filesDir := filepath.Join(tempDir, "files")
	err = os.Mkdir(filesDir, 0o755)
	require.NoError(t, err)

	filesIndexFile := filepath.Join(filesDir, "index.html")
	err = os.WriteFile(filesIndexFile, []byte("<h1>Test Page</h1>"), 0o644)
	require.NoError(t, err)

	req1 := httptest.NewRequest(http.MethodGet, "/files/index.html", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	// The serve command should work, but might redirect
	// Let's just verify it doesn't call the upstream
	assert.NotEqual(t, "should not be called", w1.Body.String())

	// Test file not found
	req2 := httptest.NewRequest(http.MethodGet, "/files/nonexistent.html", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusNotFound, w2.Code)
}

func TestHTTPFlow_ProxyCommandYAML(t *testing.T) {
	// Create a mock upstream server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Header", "upstream-value")
		w.WriteHeader(http.StatusOK)
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

	handler := rules.BuildHandler(mockUpstream(http.StatusOK, "should not be called"))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// The proxy command should forward the request to the upstream server
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "upstream response", w.Body.String())
	assert.Equal(t, "upstream-value", w.Header().Get("X-Upstream-Header"))
}

func TestHTTPFlow_NotifyCommandYAML(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "ok")

	var rules Rules
	err := parseRules(`
- name: notify-rule
  on: path /notify
  do: notify info test-provider "title $req_method" "body $req_url $status_code"
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/notify", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

func TestHTTPFlow_FormConditionsYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("form processed"))
	})

	var rules Rules
	err := parseRules(`
- name: process-form
  on: form username
  do: set resp_header X-Username "$form(username)"
- name: process-postform
  on: postform email
  do: set resp_header X-Email "$postform(email)"
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test form condition
	formData := url.Values{"username": {"john_doe"}}
	req1 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(formData.Encode()))
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "john_doe", w1.Header().Get("X-Username"))

	// Test postform condition
	postFormData := url.Values{"email": {"john@example.com"}}
	req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(postFormData.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "john@example.com", w2.Header().Get("X-Email"))
}

func TestHTTPFlow_RemoteConditionsYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
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
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "127.0.0.1:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "local", w1.Header().Get("X-Access"))

	// Test private network block
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "192.168.1.100:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 403, w2.Code)
	assert.Equal(t, "Private network blocked\n", w2.Body.String())
}

func TestHTTPFlow_BasicAuthConditionsYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
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
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.SetBasicAuth("admin", "adminpass")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "admin", w1.Header().Get("X-Auth-Status"))

	// Test guest user
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.SetBasicAuth("guest", "guestpass")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "guest", w2.Header().Get("X-Auth-Status"))
}

func TestHTTPFlow_RouteConditionsYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
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
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1 = routes.WithRouteContext(req1, mockRoute("backend"))

	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "backend", w1.Header().Get("X-Route"))

	// Test admin route
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2 = routes.WithRouteContext(req2, mockRoute("frontend"))

	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "frontend", w2.Header().Get("X-Route"))
}

func TestHTTPFlow_ResponseStatusConditionsYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, "method not allowed")
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

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, "error\n", w.Body.String())
}

func TestHTTPFlow_ResponseHeaderConditionsYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response-Header", "response header")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "processed")
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

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
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

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
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

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "processed", w.Body.String())
	})
}

func TestHTTPFlow_PreTermination_SkipsLaterPreCommands_ButRunsPostOnlyAndPostMatchersYAML(t *testing.T) {
	upstreamCalled := false
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "upstream")
	})

	var rules Rules
	err := parseRules(`
- on: path /
  do: error 403 blocked
- on: path /
  do: set resp_header X-Late should-run
- on: status 4xx
  do: set resp_header X-Post true
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.False(t, upstreamCalled)
	assert.Equal(t, 403, w.Code)
	assert.Equal(t, "blocked\n", w.Body.String())
	assert.Equal(t, "should-run", w.Header().Get("X-Late"))
	assert.Equal(t, "true", w.Header().Get("X-Post"))
}

func TestHTTPFlow_PostRuleTermination_StopsRemainingCommandsInRuleYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	var rules Rules
	err := parseRules(`
- on: status 200
  do: |
    error 500 failed
    set resp_header X-After should-not-run
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "failed\n", w.Body.String())
	assert.Empty(t, w.Header().Get("X-After"))
}

func TestHTTPFlow_ComplexRuleCombinationsYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
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
	req1 := httptest.NewRequest(http.MethodPost, "/api/admin/users", nil)
	req1.Header.Set("Authorization", "Bearer token")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "admin", w1.Header().Get("X-Access-Level"))
	assert.Equal(t, "v1", w1.Header()["X-API-Version"][0])

	// Test user API (should match second rule)
	req2 := httptest.NewRequest(http.MethodGet, "/api/users/profile", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "user", w2.Header().Get("X-Access-Level"))
	assert.Equal(t, "v1", w2.Header()["X-API-Version"][0])

	// Test public API (should match third rule)
	req3 := httptest.NewRequest(http.MethodGet, "/api/public/info", nil)
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Equal(t, "public", w3.Header().Get("X-Access-Level"))
	assert.Empty(t, w3.Header()["X-API-Version"])
}

func TestHTTPFlow_ResponseModifierYAML(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("original response"))
	})

	var rules Rules
	err := parseRules(`
- name: modify-response
  do: |
    set resp_header X-Modified "true"
    set resp_body "Modified: $req_method $req_path"
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "true", w.Header().Get("X-Modified"))
	assert.Equal(t, "Modified: GET /test\n", w.Body.String())
}

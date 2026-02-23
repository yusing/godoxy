package rules_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/route/routes"
	"golang.org/x/crypto/bcrypt"

	. "github.com/yusing/godoxy/internal/route/rules"
)

func TestHTTPFlow_BasicPreRules(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", r.Header.Get("X-Custom-Header"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream response"))
	})

	var rules Rules
	err := parseRules(`
path / {
  set header X-Custom-Header test-value
}`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "upstream response", w.Body.String())
	assert.Equal(t, "test-value", w.Header().Get("X-Custom-Header"))
}

func TestHTTPFlow_TerminatingCommand(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "should not be called")

	var rules Rules
	err := parseRules(`
path /error {
  error 403 Forbidden
}
path /error {
  set header X-Header ignored
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "Forbidden\n", w.Body.String())
	assert.Empty(t, w.Header().Get("X-Header"))
}

func TestHTTPFlow_RedirectFlow(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "should not be called")

	var rules Rules
	err := parseRules(`
path /old-path {
  redirect /new-path
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/old-path", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
	assert.Equal(t, "/new-path", w.Header().Get("Location"))
}

func TestHTTPFlow_RewriteFlow(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("path: " + r.URL.Path))
	})

	var rules Rules
	err := parseRules(`
path glob(/api/*) {
  rewrite /api/ /v1/
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "path: /v1/users", w.Body.String())
}

func TestHTTPFlow_MultiplePreRules(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream: " + r.Header.Get("X-Request-Id")))
	})

	var rules Rules
	err := parseRules(`
path / {
  set header X-Request-Id req-123
}
path / {
  set header X-Auth-Token token-456
}
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

func TestHTTPFlow_PostResponseRule(t *testing.T) {
	upstream := mockUpstreamWithHeaders(http.StatusOK, "success", http.Header{
		"X-Upstream": []string{"upstream-value"},
	})

	tempFile := TestRandomFileName()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
path /test {
  log info %s "$req_method $status_code"
}
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

func TestHTTPFlow_ResponseRuleWithStatusCondition(t *testing.T) {
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

	// Create a temporary file for logging
	tempFile := TestRandomFileName()

	err := parseRules(fmt.Sprintf(`
status 4xx {
  log error %s "$req_url returned $status_code"
}
`, tempFile), &rules)
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
	content := TestFileContent(tempFile)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	require.Len(t, lines, 1, "only 4xx requests should be logged")
	assert.Equal(t, "/notfound returned 404", lines[0])
}

func TestHTTPFlow_ConditionalRules(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello " + r.Header.Get("X-Username")))
	})

	var rules Rules
	err := parseRules(`
header Authorization {
  set header X-Username authenticated-user
  set resp_header X-Username authenticated-user
}
default {
  set header X-Username anonymous
  set resp_header X-Username anonymous
}
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

func TestHTTPFlow_ComplexFlowWithPreAndPostRules(t *testing.T) {
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

	// Create temporary files for logging
	logFile := TestRandomFileName()
	errorLogFile := TestRandomFileName()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
{
  set resp_header X-Correlation-Id random_uuid
}
path /protected {
  require_basic_auth "Protected Area"
}
{
  log info %q "$req_method $req_url -> $status_code"
}
status 4xx {
  log error %q "ERROR: $req_method $req_url $status_code"
}
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
	assert.Equal(t, w2.Body.String(), "Unauthorized\n")

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

func TestHTTPFlow_DefaultRule(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "upstream response")

	var rules Rules
	err := parseRules(`
default {
  set resp_header X-Default-Applied true
}
path /special {
  set resp_header X-Special-Handled true
}
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

func TestHTTPFlow_UnconditionalRuleSuppressesDefaultRule(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "upstream response")

	var rules Rules
	err := parseRules(`
{
  set resp_header X-Unconditional true
}
default {
  set resp_header X-Default-Applied true
}
path /never-match {
  set resp_header X-Never-Match true
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/special", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "true", w.Header().Get("X-Unconditional"))
	assert.Empty(t, w.Header().Get("X-Default-Applied"))
	assert.Empty(t, w.Header().Get("X-Never-Match"))
}

func TestHTTPFlow_HeaderManipulation(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo back a header
		headerValue := r.Header.Get("X-Test-Header")
		w.Header().Set("X-Echoed-Header", headerValue)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("header echoed"))
	})

	var rules Rules
	err := parseRules(`
{
  remove resp_header X-Secret
  add resp_header X-Custom-Header custom-value
}
header X-Test-Header {
  set header X-Test-Header modified-value
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Secret", "secret-value")
	req.Header.Set("X-Test-Header", "original-value")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "modified-value", w.Header().Get("X-Echoed-Header"))
	assert.Equal(t, "custom-value", w.Header().Get("X-Custom-Header"))
	// Ensure the secret header was removed and not passed to upstream
	// (we can't directly test this, but the upstream shouldn't see it)
}

func TestHTTPFlow_NestedBlocks_RemoteOverride(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Remote-Type", r.Header.Get("X-Remote-Type"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	var rules Rules
	err := parseRules(`
header X-Test-Header {
  set header X-Remote-Type public
  remote 127.0.0.1 | remote 192.168.0.0/16 {
    set header X-Remote-Type private
  }
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Localhost => private
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.Header.Set("X-Test-Header", "1")
	req1.RemoteAddr = "127.0.0.1:12345"
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "private", w1.Header().Get("X-Remote-Type"))

	// Public IP => public
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-Test-Header", "1")
	req2.RemoteAddr = "1.1.1.1:12345"
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "public", w2.Header().Get("X-Remote-Type"))
}

func TestHTTPFlow_NestedBlocks_ElifElse(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Mode", r.Header.Get("X-Mode"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	var rules Rules
	err := parseRules(`
header X-Test-Header {
  method GET {
    set header X-Mode get
  } elif method POST {
    set header X-Mode post
  } else {
    set header X-Mode other
  }
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// GET => get
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.Header.Set("X-Test-Header", "1")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "get", w1.Header().Get("X-Mode"))

	// POST => post
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set("X-Test-Header", "1")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "post", w2.Header().Get("X-Mode"))

	// other methods => else branch
	req3 := httptest.NewRequest(http.MethodPut, "/", nil)
	req3.Header.Set("X-Test-Header", "1")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)
	assert.Equal(t, "other", w3.Header().Get("X-Mode"))

	// no match
	req4 := httptest.NewRequest(http.MethodDelete, "/", nil)
	w4 := httptest.NewRecorder()
	handler.ServeHTTP(w4, req4)
	assert.Equal(t, http.StatusOK, w4.Code)
	assert.Equal(t, "", w4.Header().Get("X-Mode"))
}

func TestHTTPFlow_NestedBlocks_TerminatingActionStopsFlow(t *testing.T) {
	called := false
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream"))
	})

	var rules Rules
	err := parseRules(`
path / {
  set header X-Pre pre
  header X-Block {
    error 403 "blocked"
  }
  set resp_header X-After should-not-run
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Without X-Block => should reach upstream and execute non-terminating commands
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.True(t, called)
	assert.Equal(t, "should-not-run", w1.Header().Get("X-After"))
	assert.Equal(t, "pre", req1.Header.Get("X-Pre"))

	// With X-Block => nested terminating action should stop processing before upstream
	called = false
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-Block", "1")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, 403, w2.Code)
	assert.Equal(t, "blocked\n", w2.Body.String())
	assert.False(t, called, "nested error should terminate before calling upstream")
	assert.Empty(t, w2.Header().Get("X-After"), "commands after the nested block should not run")
}

func TestHTTPFlow_NestedBlocks_InResponseRule_ModifiesResponseByRequestMethod(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream"))
	})

	var rules Rules
	err := parseRules(`
{
  set header X-Method "should-be-overridden"
  method POST {
    set header X-Method "post"
  } elif method GET {
    set header X-Method "get"
  } else {
    set header X-Method "other"
  }
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	t.Run(http.MethodGet, func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "get", req.Header.Get("X-Method"))
	})

	t.Run(http.MethodPost, func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "post", req.Header.Get("X-Method"))
	})

	t.Run("other", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "other", req.Header.Get("X-Method"))
	})
}

func TestHTTPFlow_QueryParameterHandling(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("query: " + query.Get("param")))
	})

	var rules Rules
	err := parseRules(`
query param {
  set query param added-value
}
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

func TestHTTPFlow_ServeCommand(t *testing.T) {
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
path glob(/files/*) {
  serve %s
}
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

func TestHTTPFlow_ProxyCommand(t *testing.T) {
	// Create a mock upstream server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Header", "upstream-value")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream response"))
	}))
	defer upstreamServer.Close()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
path glob(/api/*) {
  proxy %s
}
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

func TestHTTPFlow_NotifyCommand(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "ok")

	var rules Rules
	err := parseRules(`
path /notify {
  notify info test-provider "title $req_method" "body $req_url $status_code"
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/notify", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

func TestHTTPFlow_FormConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("form processed"))
	})

	var rules Rules
	err := parseRules(`
form username {
  set resp_header X-Username "$form(username)"
}
postform email {
  set resp_header X-Email "$postform(email)"
}
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

func TestHTTPFlow_RemoteConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("remote processed"))
	})

	var rules Rules
	err := parseRules(`
remote 127.0.0.1 {
  set resp_header X-Access "local"
}
remote 192.168.0.0/16 {
  error 403 "Private network blocked"
}
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

func TestHTTPFlow_BasicAuthConditions(t *testing.T) {
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
basic_auth admin %q {
  set resp_header X-Auth-Status "admin"
}
basic_auth guest %q {
  set resp_header X-Auth-Status "guest"
}
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

func TestHTTPFlow_RouteConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("route processed"))
	})

	var rules Rules
	err := parseRules(`
route backend {
  set resp_header X-Route "backend"
}
route frontend {
  set resp_header X-Route "frontend"
}
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

func TestHTTPFlow_ResponseStatusConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, "method not allowed")
	})

	var rules Rules
	err := parseRules(`
status 405 {
  error 405 'error'
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, "error\n", w.Body.String())
}

func TestHTTPFlow_ResponseHeaderConditions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Response-Header", "response header")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "processed")
	})

	t.Run("any_value", func(t *testing.T) {
		var rules Rules
		err := parseRules(`
resp_header X-Response-Header {
  error 405 "error"
}
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
resp_header X-Response-Header "response header" {
  error 405 "error"
}
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
resp_header X-Response-Header "not-matched" {
  error 405 "error"
}
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

func TestHTTPFlow_ComplexRuleCombinations(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "complex processed")
	})

	var rules Rules
	err := parseRules(`
path glob(/api/admin/*) &
header Authorization &
method POST {
  set resp_header X-Access-Level "admin"
  set resp_header X-API-Version "v1"
}
path glob(/api/users/*) & method GET {
  set resp_header X-Access-Level "user"
  set resp_header X-API-Version "v1"
}
path glob(/api/public/*) & method GET {
  set resp_header X-Access-Level "public"
}
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

func TestHTTPFlow_ResponseModifier(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "original response")
	})

	var rules Rules
	err := parseRules(`{
	set resp_header X-Modified "true"
	set resp_body "Modified: $req_method $req_path"
}`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "true", w.Header().Get("X-Modified"))
	assert.Equal(t, "Modified: GET /test\n", w.Body.String())
}

func TestHTTPFlow_RequireBasicAuth_Challenge(t *testing.T) {
	called := false
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "upstream")
	})

	var rules Rules
	err := parseRules(`
path /protected {
  require_basic_auth "My Realm"
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.False(t, called, "require_basic_auth should terminate before calling upstream")
	assert.Equal(t, 401, w.Code)
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Basic")
	assert.Contains(t, w.Header().Get("WWW-Authenticate"), "My Realm")
}

func TestHTTPFlow_NegationMatcher(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "ok")

	var rules Rules
	err := parseRules(`
!path glob("/public/*") {
  set resp_header X-Scope private
}
path glob("/public/*") {
  set resp_header X-Scope public
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	t.Run("public", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/public/index.html", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "public", w.Header().Get("X-Scope"))
	})

	t.Run("private", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "private", w.Header().Get("X-Scope"))
	})
}

func TestHTTPFlow_BlockSyntaxCommentsAreIgnored(t *testing.T) {
	upstream := mockUpstream(http.StatusOK, "ok")

	var rules Rules
	err := parseRules(`
path /comment {
  // comment with braces { } should be ignored
  set resp_header X-Commented ok # trailing comment should be ignored too
  set resp_header X-Literal "//not-a-comment" // but this one is a real comment
  /* block comment
     spanning multiple lines { } */
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/comment", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Header().Get("X-Commented"))
	assert.Equal(t, "//not-a-comment", w.Header().Get("X-Literal"))
}

func TestHTTPFlow_RemoveResponseHeader_RemovesUpstreamHeader(t *testing.T) {
	upstream := mockUpstreamWithHeaders(http.StatusOK, "ok", http.Header{
		"X-Secret": []string{"top-secret"},
		"X-Keep":   []string{"keep"},
	})

	var rules Rules
	err := parseRules(`
{
  remove resp_header X-Secret
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "keep", w.Header().Get("X-Keep"))
	assert.Empty(t, w.Result().Header.Get("X-Secret"))
}

func TestHTTPFlow_RemoveRequestHeader_BeforeUpstream(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen-Secret", r.Header.Get("X-Secret"))
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	var rules Rules
	err := parseRules(`
{
  remove header X-Secret
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Secret", "should-not-reach-upstream")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("X-Seen-Secret"), "X-Secret should be removed before reaching upstream")
}

func TestHTTPFlow_RewritePreservesQueryString(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "path=%s foo=%s bar=%s", r.URL.Path, r.URL.Query().Get("foo"), r.URL.Query().Get("bar"))
	})

	var rules Rules
	err := parseRules(`
path glob("/api/*") {
  rewrite /api/ /v1/
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/api/users?foo=1&bar=2", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "path=/v1/users foo=1 bar=2", w.Body.String())
}

func TestHTTPFlow_ResponseModifier_PreservesUpstreamStatus(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "created")
	})

	var rules Rules
	err := parseRules(`
{
  set resp_body "overridden"
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodPost, "/create", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "overridden\n", w.Body.String())
}

func TestHTTPFlow_PreTermination_SkipsLaterPreCommands_ButRunsPostOnlyAndPostMatchers(t *testing.T) {
	upstreamCalled := false
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		fmt.Fprint(w, "upstream")
	})

	var rules Rules
	err := parseRules(`
path / {
  error 403 blocked
}
path / {
  set resp_header X-Late should-run
}
status 4xx {
  set resp_header X-Post true
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.False(t, upstreamCalled)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "blocked\n", w.Body.String())
	assert.Equal(t, "should-run", w.Header().Get("X-Late"))
	assert.Equal(t, "true", w.Header().Get("X-Post"))
}

func TestHTTPFlow_PostRuleTermination_StopsRemainingCommandsInRule(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})

	var rules Rules
	err := parseRules(`
status 200 {
  error 500 failed
  set resp_header X-After should-not-run
}
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

func TestHTTPFlow_EnvVarExpansionInDoBody(t *testing.T) {
	t.Setenv("GODOXY_TEST_ENV", "env-value")

	upstream := mockUpstream(http.StatusOK, "ok")

	var rules Rules
	err := parseRules(`
{
  set resp_header X-From-Env "${GODOXY_TEST_ENV}"
}
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "env-value", w.Header().Get("X-From-Env"))
}

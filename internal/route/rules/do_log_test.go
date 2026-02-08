package rules

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/serialization"
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

func parseRules(data string, target *Rules) error {
	_, err := serialization.ConvertString(data, reflect.ValueOf(target))
	return err
}

func TestLogCommand_TemporaryFile(t *testing.T) {
	upstream := mockUpstreamWithHeaders(200, "success response", http.Header{
		"Content-Type": []string{"application/json"},
	})

	logFile := TestRandomFileName()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
- name: log-request-response
  do: |
    log info %q '$req_method $req_url $status_code $resp_header(Content-Type)'
`, logFile), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("POST", "/api/users", nil)
	req.Header.Set("User-Agent", "test-agent")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "success response", w.Body.String())

	// Read and verify log content
	content := TestFileContent(logFile)
	logContent := string(content)

	assert.Equal(t, "POST /api/users 200 application/json\n", logContent)
}

func TestLogCommand_StdoutAndStderr(t *testing.T) {
	upstream := mockUpstream(200, "success")

	var rules Rules
	err := parseRules(`
- name: log-stdout
  do: |
    log info /dev/stdout "stdout: $req_method $status_code"
- name: log-stderr
  do: |
    log error /dev/stderr "stderr: $req_path $status_code"
`, &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	// Note: We can't easily capture stdout/stderr in unit tests,
	// but we can verify no errors occurred and the handler completed
}

func TestLogCommand_DifferentLogLevels(t *testing.T) {
	upstream := mockUpstream(404, "not found")

	infoFile := TestRandomFileName()
	warnFile := TestRandomFileName()
	errorFile := TestRandomFileName()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
- name: log-info
  do: |
    log info %s "INFO: $req_method $status_code"
- name: log-warn
  do: |
    log warn %s "WARN: $req_path $status_code"
- name: log-error
  do: |
    log error %s "ERROR: $req_method $req_path $status_code"
`, infoFile, warnFile, errorFile), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("DELETE", "/api/resource/123", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 404, w.Code)

	// Verify each log file
	infoContent := TestFileContent(infoFile)
	assert.Equal(t, "INFO: DELETE 404", strings.TrimSpace(string(infoContent)))

	warnContent := TestFileContent(warnFile)
	assert.Equal(t, "WARN: /api/resource/123 404", strings.TrimSpace(string(warnContent)))

	errorContent := TestFileContent(errorFile)
	assert.Equal(t, "ERROR: DELETE /api/resource/123 404", strings.TrimSpace(string(errorContent)))
}

func TestLogCommand_TemplateVariables(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("Content-Length", "42")
		w.WriteHeader(201)
		w.Write([]byte("created"))
	})

	tempFile := TestRandomFileName()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
- name: log-with-templates
  do: |
    log info %s 'Request: $req_method $req_url Host: $req_host User-Agent: $header(User-Agent) Response: $status_code Custom-Header: $resp_header(X-Custom-Header) Content-Length: $resp_header(Content-Length)'
`, tempFile), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("PUT", "/api/resource", nil)
	req.Header.Set("User-Agent", "test-client/1.0")
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 201, w.Code)

	// Verify log content
	content := TestFileContent(tempFile)
	logContent := strings.TrimSpace(string(content))

	assert.Equal(t, "Request: PUT /api/resource Host: example.com User-Agent: test-client/1.0 Response: 201 Custom-Header: custom-value Content-Length: 42", logContent)
}

func TestLogCommand_ConditionalLogging(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/error":
			w.WriteHeader(500)
			w.Write([]byte("internal server error"))
		case "/notfound":
			w.WriteHeader(404)
			w.Write([]byte("not found"))
		default:
			w.WriteHeader(200)
			w.Write([]byte("success"))
		}
	})

	successFile := TestRandomFileName()
	errorFile := TestRandomFileName()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
- name: log-success
  on: status 2xx
  do: |
    log info %q "SUCCESS: $req_method $req_path $status_code"
- name: log-error
  on: status 4xx | status 5xx
  do: |
    log error %q "ERROR: $req_method $req_path $status_code"
`, successFile, errorFile), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Test success request
	req1 := httptest.NewRequest("GET", "/success", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, 200, w1.Code)

	// Test not found request
	req2 := httptest.NewRequest("GET", "/notfound", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, 404, w2.Code)

	// Test server error request
	req3 := httptest.NewRequest("POST", "/error", nil)
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	assert.Equal(t, 500, w3.Code)

	// Verify success log
	successContent := TestFileContent(successFile)
	successLines := strings.Split(strings.TrimSpace(string(successContent)), "\n")
	assert.Len(t, successLines, 1)
	assert.Equal(t, "SUCCESS: GET /success 200", successLines[0])

	// Verify error log
	errorContent := TestFileContent(errorFile)
	errorLines := strings.Split(strings.TrimSpace(string(errorContent)), "\n")
	require.Len(t, errorLines, 2)
	assert.Equal(t, "ERROR: GET /notfound 404", errorLines[0])
	assert.Equal(t, "ERROR: POST /error 500", errorLines[1])
}

func TestLogCommand_MultipleLogEntries(t *testing.T) {
	upstream := mockUpstream(200, "response")

	tempFile := TestRandomFileName()

	var rules Rules
	err := parseRules(fmt.Sprintf(`
- name: log-multiple
  do: |
    log info %q "$req_method $req_path $status_code"`, tempFile), &rules)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	// Make multiple requests
	requests := []struct {
		method string
		path   string
	}{
		{"GET", "/users"},
		{"POST", "/users"},
		{"PUT", "/users/1"},
		{"DELETE", "/users/1"},
	}

	for _, reqInfo := range requests {
		req := httptest.NewRequest(reqInfo.method, reqInfo.path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, 200, w.Code)
	}

	// Verify all requests were logged
	content := TestFileContent(tempFile)
	logContent := strings.TrimSpace(string(content))
	lines := strings.Split(logContent, "\n")

	assert.Len(t, lines, len(requests))

	for i, reqInfo := range requests {
		expectedLog := reqInfo.method + " " + reqInfo.path + " 200"
		assert.Equal(t, expectedLog, lines[i])
	}
}

func TestLogCommand_InvalidTemplate(t *testing.T) {
	var rules Rules

	// Test with invalid template syntax
	err := parseRules(`
- name: log-invalid
  do: |
    log info /dev/stdout "$invalid_var"`, &rules)
	assert.ErrorIs(t, err, ErrUnexpectedVar)
}

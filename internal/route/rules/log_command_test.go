package rules

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogCommand_TemporaryFile(t *testing.T) {
	upstream := mockUpstreamWithHeaders(200, "success response", http.Header{
		"Content-Type": []string{"application/json"},
	})

	// Create a temporary file for logging
	tempFile, err := os.CreateTemp("", "test-log-*.log")
	require.NoError(t, err)
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	rules := Rules{
		{
			Name: "log-request-response",
			Do: Command{},
		},
	}

	err = rules[0].Do.Parse("log info " + tempFile.Name() + " {{ .Request.Method }} {{ .Request.URL.Path }} {{ .Response.StatusCode }} {{ .Response.Header.Content-Type }}")
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("POST", "/api/users", nil)
	req.Header.Set("User-Agent", "test-agent")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "success response", w.Body.String())

	// Read and verify log content
	content, err := os.ReadFile(tempFile.Name())
	require.NoError(t, err)
	logContent := string(content)

	assert.Contains(t, logContent, "POST /api/users 200 application/json")
}

func TestLogCommand_StdoutAndStderr(t *testing.T) {
	upstream := mockUpstream(200, "success")

	rules := Rules{
		{
			Name: "log-stdout",
			Do: Command{},
		},
		{
			Name: "log-stderr",
			Do: Command{},
		},
	}

	err := rules[0].Do.Parse("log info /dev/stdout \"stdout: {{ .Request.Method }} {{ .Response.StatusCode }}\"")
	require.NoError(t, err)

	err = rules[1].Do.Parse("log error /dev/stderr \"stderr: {{ .Request.URL.Path }} {{ .Response.StatusCode }}\"")
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

	// Create temporary files for different log levels
	infoFile, err := os.CreateTemp("", "test-info-*.log")
	require.NoError(t, err)
	infoFile.Close()
	defer os.Remove(infoFile.Name())

	warnFile, err := os.CreateTemp("", "test-warn-*.log")
	require.NoError(t, err)
	warnFile.Close()
	defer os.Remove(warnFile.Name())

	errorFile, err := os.CreateTemp("", "test-error-*.log")
	require.NoError(t, err)
	errorFile.Close()
	defer os.Remove(errorFile.Name())

	rules := Rules{
		{
			Name: "log-info",
			Do: Command{},
		},
		{
			Name: "log-warn",
			Do: Command{},
		},
		{
			Name: "log-error",
			Do: Command{},
		},
	}

	err = rules[0].Do.Parse("log info " + infoFile.Name() + " INFO: {{ .Request.Method }} {{ .Response.StatusCode }}")
	require.NoError(t, err)

	err = rules[1].Do.Parse("log warn " + warnFile.Name() + " WARN: {{ .Request.URL.Path }} {{ .Response.StatusCode }}")
	require.NoError(t, err)

	err = rules[2].Do.Parse("log error " + errorFile.Name() + " ERROR: {{ .Request.Method }} {{ .Request.URL.Path }} {{ .Response.StatusCode }}")
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("DELETE", "/api/resource/123", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 404, w.Code)

	// Verify each log file
	infoContent, err := os.ReadFile(infoFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(infoContent), "INFO: DELETE 404")

	warnContent, err := os.ReadFile(warnFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(warnContent), "WARN: /api/resource/123 404")

	errorContent, err := os.ReadFile(errorFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(errorContent), "ERROR: DELETE /api/resource/123 404")
}

func TestLogCommand_TemplateVariables(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("Content-Length", "42")
		w.WriteHeader(201)
		w.Write([]byte("created"))
	})

	// Create temporary file
	tempFile, err := os.CreateTemp("", "test-template-*.log")
	require.NoError(t, err)
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	rules := Rules{
		{
			Name: "log-with-templates",
			Do: Command{},
		},
	}

	// Test various template variables
	template := `Request: {{ .Request.Method }} {{ .Request.URL }}
Host: {{ .Request.Host }}
User-Agent: {{ .Request.Header.User-Agent }}
Response: {{ .Response.StatusCode }}
Custom-Header: {{ .Response.Header.X-Custom-Header }}
Content-Length: {{ .Response.Header.Content-Length }}`

	err = rules[0].Do.Parse("log info " + tempFile.Name() + " " + template)
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("PUT", "/api/resource", nil)
	req.Header.Set("User-Agent", "test-client/1.0")
	req.Host = "example.com"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 201, w.Code)

	// Verify log content
	content, err := os.ReadFile(tempFile.Name())
	require.NoError(t, err)
	logContent := string(content)

	assert.Contains(t, logContent, "Request: PUT /api/resource")
	assert.Contains(t, logContent, "Host: example.com")
	assert.Contains(t, logContent, "User-Agent: test-client/1.0")
	assert.Contains(t, logContent, "Response: 201")
	assert.Contains(t, logContent, "Custom-Header: custom-value")
	assert.Contains(t, logContent, "Content-Length: 42")
}

func TestLogCommand_ConditionalLogging(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/error" {
			w.WriteHeader(500)
			w.Write([]byte("internal server error"))
		} else if r.URL.Path == "/notfound" {
			w.WriteHeader(404)
			w.Write([]byte("not found"))
		} else {
			w.WriteHeader(200)
			w.Write([]byte("success"))
		}
	})

	// Create temporary files
	successFile, err := os.CreateTemp("", "test-success-*.log")
	require.NoError(t, err)
	successFile.Close()
	defer os.Remove(successFile.Name())

	errorFile, err := os.CreateTemp("", "test-error-*.log")
	require.NoError(t, err)
	errorFile.Close()
	defer os.Remove(errorFile.Name())

	rules := Rules{
		{
			Name: "log-success",
			On: RuleOn{},
			Do: Command{},
		},
		{
			Name: "log-errors",
			On: RuleOn{},
			Do: Command{},
		},
	}

	// Log only 2xx responses
	err = rules[0].On.Parse("status 2xx")
	require.NoError(t, err)
	err = rules[0].Do.Parse("log info " + successFile.Name() + " SUCCESS: {{ .Request.Method }} {{ .Request.URL.Path }} {{ .Response.StatusCode }}")
	require.NoError(t, err)

	// Log only 4xx and 5xx responses
	err = rules[1].On.Parse("status 4xx | status 5xx")
	require.NoError(t, err)
	err = rules[1].Do.Parse("log error " + errorFile.Name() + " ERROR: {{ .Request.Method }} {{ .Request.URL.Path }} {{ .Response.StatusCode }}")
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
	successContent, err := os.ReadFile(successFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(successContent), "SUCCESS: GET /success 200")
	assert.NotContains(t, string(successContent), "ERROR:")
	assert.NotContains(t, string(successContent), "404")
	assert.NotContains(t, string(successContent), "500")

	// Verify error log
	errorContent, err := os.ReadFile(errorFile.Name())
	require.NoError(t, err)
	assert.NotContains(t, string(errorContent), "SUCCESS:")
	assert.Contains(t, string(errorContent), "ERROR: GET /notfound 404")
	assert.Contains(t, string(errorContent), "ERROR: POST /error 500")
}

func TestLogCommand_MultipleLogEntries(t *testing.T) {
	upstream := mockUpstream(200, "response")

	// Create temporary file
	tempFile, err := os.CreateTemp("", "test-multiple-*.log")
	require.NoError(t, err)
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	rules := Rules{
		{
			Name: "log-every-request",
			Do: Command{},
		},
	}

	err = rules[0].Do.Parse("log info " + tempFile.Name() + " {{ .Request.Method }} {{ .Request.URL.Path }} {{ .Response.StatusCode }}")
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
	content, err := os.ReadFile(tempFile.Name())
	require.NoError(t, err)
	logContent := string(content)

	for _, reqInfo := range requests {
		expectedLog := reqInfo.method + " " + reqInfo.path + " 200"
		assert.Contains(t, logContent, expectedLog)
	}

	// Count the number of log entries
	lines := strings.Split(strings.TrimSpace(logContent), "\n")
	assert.Equal(t, len(requests), len(lines))
}

func TestLogCommand_FilePermissions(t *testing.T) {
	upstream := mockUpstream(200, "success")

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "test-log-dir")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a log file path within the temp directory
	logFilePath := filepath.Join(tempDir, "test.log")

	rules := Rules{
		{
			Name: "log-to-file",
			Do: Command{},
		},
	}

	err = rules[0].Do.Parse("log info " + logFilePath + " {{ .Request.Method }} {{ .Response.StatusCode }}")
	require.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	// Verify file was created and is writable
	_, err = os.Stat(logFilePath)
	require.NoError(t, err)

	// Test writing to the file again to ensure it's not closed
	req2 := httptest.NewRequest("POST", "/test2", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, 200, w2.Code)

	// Verify both entries are in the file
	content, err := os.ReadFile(logFilePath)
	require.NoError(t, err)
	logContent := string(content)

	assert.Contains(t, logContent, "GET 200")
	assert.Contains(t, logContent, "POST 200")
}

func TestLogCommand_InvalidTemplate(t *testing.T) {
	upstream := mockUpstream(200, "success")

	rules := Rules{
		{
			Name: "log-with-invalid-template",
			Do: Command{},
		},
	}

	// Test with invalid template syntax
	err := rules[0].Do.Parse("log info /dev/stdout \"{{ .Invalid.Field }}\"")
	// Should not error during parsing, but template execution will fail gracefully
	assert.NoError(t, err)

	handler := rules.BuildHandler(upstream)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	// Should not panic
	assert.NotPanics(t, func() {
		handler.ServeHTTP(w, req)
	})

	assert.Equal(t, 200, w.Code)
}
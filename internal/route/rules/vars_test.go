package rules

import (
	"bytes"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	httputils "github.com/yusing/goutils/http"
)

func TestExtractArgs(t *testing.T) {
	tests := []struct {
		name        string
		src         string
		startPos    int
		funcName    string
		wantArgs    []string
		wantNextIdx int
		wantErr     bool
	}{
		{
			name:        "unquoted single arg",
			src:         "header(X-Some-Header)",
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"X-Some-Header"},
			wantNextIdx: 20,
		},
		{
			name:        "double quoted arg",
			src:         `header("X-Some-Header")`,
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"X-Some-Header"},
			wantNextIdx: 22,
		},
		{
			name:        "single quoted arg",
			src:         "header('X-Some-Header')",
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"X-Some-Header"},
			wantNextIdx: 22,
		},
		{
			name:        "backtick quoted arg",
			src:         "header(`X-Some-Header`)",
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"X-Some-Header"},
			wantNextIdx: 22,
		},
		{
			name:        "two args with double quotes and unquoted",
			src:         `header("X-Some-Header", 1)`,
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"X-Some-Header", "1"},
			wantNextIdx: 25,
		},
		{
			name:        "two args with single and double quotes",
			src:         "header('X-Some-Header', \"1\")",
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"X-Some-Header", "1"},
			wantNextIdx: 27,
		},
		{
			name:        "two args with backtick and single quotes",
			src:         "header(`X-Some-Header`, '1')",
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"X-Some-Header", "1"},
			wantNextIdx: 27,
		},
		{
			name:        "quoted string with nested different quotes",
			src:         `arg("'(value)'")`,
			startPos:    0,
			funcName:    "arg",
			wantArgs:    []string{"'(value)'"},
			wantNextIdx: 15,
		},
		{
			name:        "quoted string with backticks inside double quotes",
			src:         "header(\"value`with`backticks\")",
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"value`with`backticks"},
			wantNextIdx: 29,
		},
		{
			name:        "multiple args with whitespace",
			src:         "header(  \"X-Header\"  ,  2  )",
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"X-Header", "2"},
			wantNextIdx: 27,
		},
		{
			name:        "empty quoted string",
			src:         `header("")`,
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{""},
			wantNextIdx: 9,
		},
		{
			name:        "multiple empty args",
			src:         `header("", "")`,
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"", ""},
			wantNextIdx: 13,
		},
		{
			name:        "unquoted args separated by comma",
			src:         "header(key1,key2,key3)",
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"key1", "key2", "key3"},
			wantNextIdx: 21,
		},
		{
			name:        "trailing whitespace before closing paren",
			src:         `header("value"  )`,
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"value"},
			wantNextIdx: 16,
		},
		{
			name:        "startPos not at beginning",
			src:         "prefix_header(X-Header)",
			startPos:    7,
			funcName:    "header",
			wantArgs:    []string{"X-Header"},
			wantNextIdx: 22,
		},
		{
			name:        "special chars in unquoted arg",
			src:         "header(X-Custom_Header.v1)",
			startPos:    0,
			funcName:    "header",
			wantArgs:    []string{"X-Custom_Header.v1"},
			wantNextIdx: 25,
		},
		{
			name:     "unterminated quote",
			src:      `header("X-Header`,
			startPos: 0,
			funcName: "header",
			wantErr:  true,
		},
		{
			name:     "missing closing parenthesis",
			src:      `header("X-Header"`,
			startPos: 0,
			funcName: "header",
			wantErr:  true,
		},
		{
			name:     "no opening parenthesis",
			src:      `header"X-Header"`,
			startPos: 0,
			funcName: "header",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, nextIdx, err := extractArgs(tt.src, tt.startPos, tt.funcName)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantArgs, args)
				require.Equal(t, tt.wantNextIdx, nextIdx)
			}
		})
	}
}

func TestExpandVars(t *testing.T) {
	// Create a comprehensive test request with form data
	formData := url.Values{}
	formData.Set("field1", "value1")
	formData.Set("field2", "value2")
	formData.Add("multi", "first")
	formData.Add("multi", "second")

	postFormData := url.Values{}
	postFormData.Set("postfield1", "postvalue1")
	postFormData.Set("postfield2", "postvalue2")
	postFormData.Add("postmulti", "first")
	postFormData.Add("postmulti", "second")

	testRequest := httptest.NewRequest(http.MethodPost, "https://example.com:8080/api/users?param1=value1&param2=value2#fragment", strings.NewReader(postFormData.Encode()))
	testRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	testRequest.Header.Set("User-Agent", "test-agent/1.0")
	testRequest.Header.Add("X-Custom", "value1")
	testRequest.Header.Add("X-Custom", "value2")
	testRequest.ContentLength = 12345
	testRequest.RemoteAddr = "192.168.1.100:54321"
	testRequest.Form = formData
	// ParseForm to populate PostForm from the request body
	testRequest.PostForm = postFormData

	// Create response modifier with headers
	testResponseModifier := httputils.NewResponseModifier(httptest.NewRecorder())
	testResponseModifier.Header().Set("Content-Type", "text/html")
	testResponseModifier.Header().Set("X-Custom-Resp", "resp-value")
	testResponseModifier.WriteHeader(http.StatusOK)
	// set content length to 9876 by writing 9876 'a' bytes
	testResponseModifier.Write(bytes.Repeat([]byte("a"), 9876))

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// Basic request variables
		{
			name:  "req_method",
			input: "$req_method",
			want:  http.MethodPost,
		},
		{
			name:  "req_path",
			input: "$req_path",
			want:  "/api/users",
		},
		{
			name:  "req_query",
			input: "$req_query",
			want:  "param1=value1&param2=value2",
		},
		{
			name:  "req_url",
			input: "$req_url",
			want:  "https://example.com:8080/api/users?param1=value1&param2=value2#fragment",
		},
		{
			name:  "req_uri",
			input: "$req_uri",
			want:  "/api/users?param1=value1&param2=value2",
		},
		{
			name:  "req_host",
			input: "$req_host",
			want:  "example.com",
		},
		{
			name:  "req_port",
			input: "$req_port",
			want:  "8080",
		},
		{
			name:  "req_addr",
			input: "$req_addr",
			want:  "example.com:8080",
		},
		{
			name:  "req_content_type",
			input: "$req_content_type",
			want:  "application/x-www-form-urlencoded",
		},
		{
			name:  "req_content_length",
			input: "$req_content_length",
			want:  "12345",
		},
		{
			name:  "remote_host",
			input: "$remote_host",
			want:  "192.168.1.100",
		},
		{
			name:  "remote_port",
			input: "$remote_port",
			want:  "54321",
		},
		{
			name:  "remote_addr",
			input: "$remote_addr",
			want:  "192.168.1.100:54321",
		},
		// Response variables
		{
			name:  "status_code",
			input: "$status_code",
			want:  "200",
		},
		{
			name:  "resp_content_type",
			input: "$resp_content_type",
			want:  "text/html",
		},
		{
			name:  "resp_content_length",
			input: "$resp_content_length",
			want:  "9876",
		},
		// Function-like variables - header
		{
			name:  "header single value",
			input: "$header(User-Agent)",
			want:  "test-agent/1.0",
		},
		{
			name:  "header with index 0",
			input: "$header(X-Custom, 0)",
			want:  "value1",
		},
		{
			name:  "header with index 1",
			input: "$header(X-Custom, 1)",
			want:  "value2",
		},
		{
			name:  "header not found",
			input: "$header(X-Not-Found)",
			want:  "",
		},
		{
			name:  "header index out of range",
			input: "$header(X-Custom, 99)",
			want:  "",
		},
		// Function-like variables - resp_header
		{
			name:  "resp_header single value",
			input: "$resp_header(Content-Type)",
			want:  "text/html",
		},
		{
			name:  "resp_header custom header",
			input: "$resp_header(X-Custom-Resp)",
			want:  "resp-value",
		},
		{
			name:  "resp_header not found",
			input: "$resp_header(X-Not-Found)",
			want:  "",
		},
		// Function-like variables - arg (query parameters)
		{
			name:  "arg single parameter",
			input: "$arg(param1)",
			want:  "value1",
		},
		{
			name:  "arg second parameter",
			input: "$arg(param2)",
			want:  "value2",
		},
		{
			name:  "arg not found",
			input: "$arg(param3)",
			want:  "",
		},
		// Function-like variables - form
		{
			name:  "form single parameter",
			input: "$form(field1)",
			want:  "value1",
		},
		{
			name:  "form second parameter",
			input: "$form(field2)",
			want:  "value2",
		},
		{
			name:  "form multi-value first",
			input: "$form(multi, 0)",
			want:  "first",
		},
		{
			name:  "form multi-value second",
			input: "$form(multi, 1)",
			want:  "second",
		},
		{
			name:  "form not found",
			input: "$form(nonexistent)",
			want:  "",
		},
		{
			name:  "form index out of range",
			input: "$form(field1, 10)",
			want:  "",
		},
		// Function-like variables - postform
		{
			name:  "postform single parameter",
			input: "$postform(postfield1)",
			want:  "postvalue1",
		},
		{
			name:  "postform second parameter",
			input: "$postform(postfield2)",
			want:  "postvalue2",
		},
		{
			name:  "postform multi-value first",
			input: "$postform(postmulti, 0)",
			want:  "first",
		},
		{
			name:  "postform multi-value second",
			input: "$postform(postmulti, 1)",
			want:  "second",
		},
		{
			name:  "postform not found",
			input: "$postform(nonexistent)",
			want:  "",
		},
		{
			name:  "postform index out of range",
			input: "$postform(postfield1, 10)",
			want:  "",
		},
		// Mixed variables
		{
			name:  "mixed variables",
			input: "$req_method $req_path $status_code",
			want:  "POST /api/users 200",
		},
		{
			name:  "variables with text",
			input: "Method: $req_method, Path: $req_path",
			want:  "Method: POST, Path: /api/users",
		},
		{
			name:  "function variables with text",
			input: "Header: $header(User-Agent), Status: $status_code",
			want:  "Header: test-agent/1.0, Status: 200",
		},
		// Escaped dollar signs
		{
			name:  "escaped dollar",
			input: "$$req_method",
			want:  "$req_method",
		},
		{
			name:  "mixed escaped and unescaped",
			input: "$$req_method $req_path",
			want:  "$req_method /api/users",
		},
		// Environment variable syntax ${}
		{
			name:  "env var syntax",
			input: "${VAR}",
			want:  "${VAR}",
		},
		// Error cases
		{
			name:    "unknown variable",
			input:   "$unknown_var",
			wantErr: true,
		},
		{
			name:    "invalid function syntax",
			input:   "$arg(param1",
			wantErr: true,
		},
		{
			name:    "incomplete dollar",
			input:   "test$",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			_, err := ExpandVars(testResponseModifier, testRequest, tt.input, &out)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, out.String())
			}
		})
	}
}

func TestExpandVars_Integration(t *testing.T) {
	t.Run("complex log format", func(t *testing.T) {
		testRequest := httptest.NewRequest(http.MethodGet, "https://api.example.com/users/123?sort=asc", nil)
		testRequest.Header.Set("User-Agent", "curl/7.68.0")
		testRequest.RemoteAddr = "10.0.0.1:54321"

		testResponseModifier := httputils.NewResponseModifier(httptest.NewRecorder())
		testResponseModifier.WriteHeader(http.StatusOK)

		var out strings.Builder
		_, err := ExpandVars(testResponseModifier, testRequest,
			"$req_method $req_url $status_code User-Agent=$header(User-Agent)",
			&out)

		require.NoError(t, err)
		require.Equal(t, "GET https://api.example.com/users/123?sort=asc 200 User-Agent=curl/7.68.0", out.String())
	})

	t.Run("with query parameters", func(t *testing.T) {
		testRequest := httptest.NewRequest(http.MethodGet, "http://example.com/search?q=test&page=1", nil)

		testResponseModifier := httputils.NewResponseModifier(httptest.NewRecorder())

		var out strings.Builder
		_, err := ExpandVars(testResponseModifier, testRequest,
			"Query: $arg(q), Page: $arg(page)",
			&out)

		require.NoError(t, err)
		require.Equal(t, "Query: test, Page: 1", out.String())
	})

	t.Run("response headers", func(t *testing.T) {
		testRequest := httptest.NewRequest(http.MethodGet, "/", nil)

		testResponseModifier := httputils.NewResponseModifier(httptest.NewRecorder())
		testResponseModifier.Header().Set("Cache-Control", "no-cache")
		testResponseModifier.Header().Set("X-Rate-Limit", "100")
		testResponseModifier.WriteHeader(http.StatusOK)

		var out strings.Builder
		_, err := ExpandVars(testResponseModifier, testRequest,
			"Status: $status_code, Cache: $resp_header(Cache-Control), Limit: $resp_header(X-Rate-Limit)",
			&out)

		require.NoError(t, err)
		require.Equal(t, "Status: 200, Cache: no-cache, Limit: 100", out.String())
	})
}

func TestExpandVars_RequestSchemes(t *testing.T) {
	tests := []struct {
		name     string
		request  *http.Request
		expected string
	}{
		{
			name:     "http scheme",
			request:  httptest.NewRequest(http.MethodGet, "http://example.com/", nil),
			expected: "http",
		},
		{
			name: "https scheme",
			request: &http.Request{
				Method: http.MethodGet,
				URL:    &url.URL{Scheme: "https", Host: "example.com", Path: "/"},
				TLS:    &tls.ConnectionState{}, // Simulate TLS connection
			},
			expected: "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testResponseModifier := httputils.NewResponseModifier(httptest.NewRecorder())
			var out strings.Builder
			_, err := ExpandVars(testResponseModifier, tt.request, "$req_scheme", &out)
			require.NoError(t, err)
			require.Equal(t, tt.expected, out.String())
		})
	}
}

func TestExpandVars_UpstreamVariables(t *testing.T) {
	// Upstream variables require context from routes package
	testRequest := httptest.NewRequest(http.MethodGet, "/", nil)

	testResponseModifier := httputils.NewResponseModifier(httptest.NewRecorder())

	// Test that upstream variables don't cause errors even when not set
	upstreamVars := []string{
		"$upstream_name",
		"$upstream_scheme",
		"$upstream_host",
		"$upstream_port",
		"$upstream_addr",
		"$upstream_url",
	}

	for _, varExpr := range upstreamVars {
		t.Run(varExpr, func(t *testing.T) {
			var out strings.Builder
			_, err := ExpandVars(testResponseModifier, testRequest, varExpr, &out)
			// Should not error, may return empty string
			require.NoError(t, err)
		})
	}
}

func TestExpandVars_NoHostPort(t *testing.T) {
	// Test request without port in Host header
	testRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	testRequest.Host = "example.com" // No port

	testResponseModifier := httputils.NewResponseModifier(httptest.NewRecorder())

	t.Run("req_host without port", func(t *testing.T) {
		var out strings.Builder
		_, err := ExpandVars(testResponseModifier, testRequest, "$req_host", &out)
		require.NoError(t, err)
		require.Equal(t, "example.com", out.String())
	})

	t.Run("req_port without port", func(t *testing.T) {
		var out strings.Builder
		_, err := ExpandVars(testResponseModifier, testRequest, "$req_port", &out)
		require.NoError(t, err)
		require.Equal(t, "", out.String())
	})
}

func TestExpandVars_NoRemotePort(t *testing.T) {
	// Test request without port in RemoteAddr
	testRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	testRequest.RemoteAddr = "192.168.1.1" // No port

	testResponseModifier := httputils.NewResponseModifier(httptest.NewRecorder())

	t.Run("remote_host without port", func(t *testing.T) {
		var out strings.Builder
		_, err := ExpandVars(testResponseModifier, testRequest, "$remote_host", &out)
		require.NoError(t, err)
		require.Equal(t, "", out.String())
	})

	t.Run("remote_port without port", func(t *testing.T) {
		var out strings.Builder
		_, err := ExpandVars(testResponseModifier, testRequest, "$remote_port", &out)
		require.NoError(t, err)
		require.Equal(t, "", out.String())
	})
}

func TestExpandVars_WhitespaceHandling(t *testing.T) {
	testRequest := httptest.NewRequest(http.MethodGet, "/test", nil)
	testResponseModifier := httputils.NewResponseModifier(httptest.NewRecorder())

	var out strings.Builder
	_, err := ExpandVars(testResponseModifier, testRequest, "$req_method $req_path", &out)
	require.NoError(t, err)
	require.Equal(t, "GET /test", out.String())
}

func TestValidateVars(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "valid simple variable",
			input: "$req_method",
		},
		{
			name:  "valid function variable",
			input: "$header(User-Agent)",
		},
		{
			name:  "valid response variable",
			input: "$status_code",
		},
		{
			name:    "invalid variable",
			input:   "$unknown_var",
			wantErr: true,
		},
		{
			name:    "incomplete variable",
			input:   "test$",
			wantErr: true,
		},
		{
			name:  "valid variables with text",
			input: "Method: $req_method",
		},
		{
			name:  "valid escaped dollar",
			input: "$$req_method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateVars(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNeedExpandVars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "contains variable",
			input: "$req_method",
			want:  true,
		},
		{
			name:  "contains function variable",
			input: "$header(X-Test)",
			want:  true,
		},
		{
			name:  "no variable",
			input: "plain text",
			want:  false,
		},
		{
			name:  "escaped dollar",
			input: "$$req_method",
			want:  true,
		},
		{
			name:  "mixed content",
			input: "Method: $req_method",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedExpandVars(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

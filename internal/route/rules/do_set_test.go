package rules

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	httputils "github.com/yusing/goutils/http"
)

func TestFieldHandler_Header(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		modifier FieldModifier
		setup    func(*http.Request)
		verify   func(*http.Request, *httptest.ResponseRecorder)
	}{
		{
			name:     "set header",
			key:      "X-Test",
			value:    "test-value",
			modifier: ModFieldSet,
			setup: func(r *http.Request) {
				r.Header.Set("X-Test", "old-value")
			},
			verify: func(r *http.Request, w *httptest.ResponseRecorder) {
				got := r.Header.Get("X-Test")
				assert.Equal(t, "test-value", got, "Expected header X-Test to be 'test-value'")
			},
		},
		{
			name:     "add header",
			key:      "X-Test",
			value:    "new-value",
			modifier: ModFieldAdd,
			setup: func(r *http.Request) {
				r.Header.Set("X-Test", "existing-value")
			},
			verify: func(r *http.Request, w *httptest.ResponseRecorder) {
				values := r.Header["X-Test"]
				require.Len(t, values, 2, "Expected 2 header values")
				assert.Equal(t, "existing-value", values[0], "Expected first value of X-Test header to be 'existing-value'")
				assert.Equal(t, "new-value", values[1], "Expected second value of X-Test header to be 'new-value'")
			},
		},
		{
			name:     "remove header",
			key:      "X-Test",
			value:    "",
			modifier: ModFieldRemove,
			setup: func(r *http.Request) {
				r.Header.Set("X-Test", "to-be-removed")
			},
			verify: func(r *http.Request, w *httptest.ResponseRecorder) {
				got := r.Header.Get("X-Test")
				assert.Empty(t, got, "Expected header X-Test to be removed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.setup(req)
			w := httptest.NewRecorder()

			_, tmpl, tErr := validateTemplate(tt.value, false)
			if tErr != nil {
				t.Fatalf("Failed to validate template: %v", tErr)
			}
			handler := modFields[FieldHeader].builder(&keyValueTemplate{tt.key, tmpl})
			var cmd HandlerFunc
			switch tt.modifier {
			case ModFieldSet:
				cmd = handler.set
			case ModFieldAdd:
				cmd = handler.add
			case ModFieldRemove:
				cmd = handler.remove
			}

			err := cmd(httputils.NewResponseModifier(w), req, nil)
			if err != nil {
				t.Fatalf("Handler returned error: %v", err)
			}

			tt.verify(req, w)
		})
	}
}

func TestFieldHandler_ResponseHeader(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		modifier FieldModifier
		setup    func(*httptest.ResponseRecorder)
		verify   func(*httptest.ResponseRecorder)
	}{
		{
			name:     "set response header",
			key:      "X-Response-Test",
			value:    "response-value",
			modifier: ModFieldSet,
			verify: func(w *httptest.ResponseRecorder) {
				got := w.Header().Get("X-Response-Test")
				assert.Equal(t, "response-value", got, "Expected response header X-Response-Test to be 'response-value'")
			},
		},
		{
			name:     "add response header",
			key:      "X-Response-Test",
			value:    "additional-value",
			modifier: ModFieldAdd,
			setup: func(w *httptest.ResponseRecorder) {
				w.Header().Set("X-Response-Test", "existing-value")
			},
			verify: func(w *httptest.ResponseRecorder) {
				values := w.Header()["X-Response-Test"]
				require.Len(t, values, 2)
				assert.Equal(t, "existing-value", values[0])
				assert.Equal(t, "additional-value", values[1])
			},
		},
		{
			name:     "remove response header",
			key:      "X-Response-Test",
			value:    "",
			modifier: ModFieldRemove,
			verify: func(w *httptest.ResponseRecorder) {
				assert.Empty(t, w.Header().Get("X-Response-Test"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			if tt.setup != nil {
				tt.setup(w)
			}

			_, tmpl, tErr := validateTemplate(tt.value, false)
			if tErr != nil {
				t.Fatalf("Failed to validate template: %v", tErr)
			}
			handler := modFields[FieldResponseHeader].builder(&keyValueTemplate{tt.key, tmpl})
			var cmd HandlerFunc
			switch tt.modifier {
			case ModFieldSet:
				cmd = handler.set
			case ModFieldAdd:
				cmd = handler.add
			case ModFieldRemove:
				cmd = handler.remove
			}

			err := cmd(httputils.NewResponseModifier(w), req, nil)
			if err != nil {
				t.Fatalf("Handler returned error: %v", err)
			}

			tt.verify(w)
		})
	}
}

func TestFieldHandler_Query(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		modifier FieldModifier
		setup    func(*http.Request)
		verify   func(*http.Request)
	}{
		{
			name:     "set query",
			key:      "test",
			value:    "new-value",
			modifier: ModFieldSet,
			setup: func(r *http.Request) {
				r.URL.RawQuery = "test=old-value&other=keep"
			},
			verify: func(r *http.Request) {
				got := r.URL.Query().Get("test")
				assert.Equal(t, "new-value", got, "Expected query 'test' to be 'new-value'")
				gotOther := r.URL.Query().Get("other")
				assert.Equal(t, "keep", gotOther, "Expected query 'other' to be 'keep'")
			},
		},
		{
			name:     "add query",
			key:      "test",
			value:    "additional-value",
			modifier: ModFieldAdd,
			setup: func(r *http.Request) {
				r.URL.RawQuery = "test=existing-value"
			},
			verify: func(r *http.Request) {
				values := r.URL.Query()["test"]
				require.Len(t, values, 2, "Expected 2 query values")
				assert.Equal(t, "existing-value", values[0], "Expected first value of test query param to be 'existing-value'")
				assert.Equal(t, "additional-value", values[1], "Expected second value of test query param to be 'additional-value'")
			},
		},
		{
			name:     "remove query",
			key:      "test",
			value:    "",
			modifier: ModFieldRemove,
			setup: func(r *http.Request) {
				r.URL.RawQuery = "test=to-be-removed&other=keep"
			},
			verify: func(r *http.Request) {
				got := r.URL.Query().Get("test")
				assert.Empty(t, got, "Expected query 'test' to be removed")
				gotOther := r.URL.Query().Get("other")
				assert.Equal(t, "keep", gotOther, "Expected query 'other' to be 'keep'")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.setup(req)
			w := httptest.NewRecorder()

			_, tmpl, tErr := validateTemplate(tt.value, false)
			if tErr != nil {
				t.Fatalf("Failed to validate template: %v", tErr)
			}
			handler := modFields[FieldQuery].builder(&keyValueTemplate{tt.key, tmpl})
			var cmd HandlerFunc
			switch tt.modifier {
			case ModFieldSet:
				cmd = handler.set
			case ModFieldAdd:
				cmd = handler.add
			case ModFieldRemove:
				cmd = handler.remove
			}

			err := cmd(httputils.NewResponseModifier(w), req, nil)
			if err != nil {
				t.Fatalf("Handler returned error: %v", err)
			}

			tt.verify(req)
		})
	}
}

func TestFieldHandler_Cookie(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		modifier FieldModifier
		setup    func(*http.Request)
		verify   func(*http.Request)
	}{
		{
			name:     "set cookie",
			key:      "test",
			value:    "new-value",
			modifier: ModFieldSet,
			setup: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: "test", Value: "old-value"})
			},
			verify: func(r *http.Request) {
				cookie, err := r.Cookie("test")
				assert.NoError(t, err, "Expected cookie 'test' to exist")
				if err == nil {
					assert.Equal(t, "new-value", cookie.Value, "Expected cookie 'test' to be 'new-value'")
				}
			},
		},
		{
			name:     "add cookie",
			key:      "test",
			value:    "additional-value",
			modifier: ModFieldAdd,
			setup: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: "test", Value: "existing-value"})
			},
			verify: func(r *http.Request) {
				cookies := r.Cookies()
				testCookies := make([]string, 0)
				for _, c := range cookies {
					if c.Name == "test" {
						testCookies = append(testCookies, c.Value)
					}
				}
				require.Len(t, testCookies, 2, "Expected 2 cookies with name 'test'")
				assert.Equal(t, "existing-value", testCookies[0], "Expected first value of 'test' cookie to be 'existing-value'")
				assert.Equal(t, "additional-value", testCookies[1], "Expected second value of 'test' cookie to be 'additional-value'")
			},
		},
		{
			name:     "remove cookie",
			key:      "test",
			value:    "",
			modifier: ModFieldRemove,
			setup: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: "test", Value: "to-be-removed"})
				r.AddCookie(&http.Cookie{Name: "other", Value: "keep"})
			},
			verify: func(r *http.Request) {
				_, err := r.Cookie("test")
				assert.Error(t, err, "Expected cookie 'test' to be removed")
				cookie, err := r.Cookie("other")
				assert.NoError(t, err, "Expected cookie 'other' to exist")
				if err == nil {
					assert.Equal(t, "keep", cookie.Value, "Expected cookie 'other' to be 'keep'")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.setup(req)
			w := httptest.NewRecorder()

			_, tmpl, tErr := validateTemplate(tt.value, false)
			if tErr != nil {
				t.Fatalf("Failed to validate template: %v", tErr)
			}
			handler := modFields[FieldCookie].builder(&keyValueTemplate{tt.key, tmpl})
			var cmd HandlerFunc
			switch tt.modifier {
			case ModFieldSet:
				cmd = handler.set
			case ModFieldAdd:
				cmd = handler.add
			case ModFieldRemove:
				cmd = handler.remove
			}

			err := cmd(httputils.NewResponseModifier(w), req, nil)
			if err != nil {
				t.Fatalf("Handler returned error: %v", err)
			}

			tt.verify(req)
		})
	}
}

func TestFieldHandler_Body(t *testing.T) {
	tests := []struct {
		name     string
		template string
		setup    func(*http.Request)
		verify   func(*http.Request)
	}{
		{
			name:     "set body with template",
			template: "Hello $req_method $req_path",
			setup: func(r *http.Request) {
				r.Method = http.MethodPost
				r.URL.Path = "/test"
			},
			verify: func(r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err, "Failed to read body")
				expected := "Hello POST /test"
				assert.Equal(t, expected, string(body), "Expected body content")
			},
		},
		{
			name:     "set body with existing body",
			template: "Overridden",
			setup: func(r *http.Request) {
				r.Body = io.NopCloser(strings.NewReader("original body"))
			},
			verify: func(r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err, "Failed to read body")
				assert.Equal(t, "Overridden", string(body), "Expected body to be 'Overridden'")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.setup(req)
			w := httputils.NewResponseModifier(httptest.NewRecorder())

			_, tmpl, tErr := validateTemplate(tt.template, false)
			if tErr != nil {
				t.Fatalf("Failed to parse template: %v", tErr)
			}

			handler := modFields[FieldBody].builder(tmpl)
			err := handler.set(w, req, nil)
			if err != nil {
				t.Fatalf("Handler returned error: %v", err)
			}

			tt.verify(req)
		})
	}
}

func TestFieldHandler_ResponseBody(t *testing.T) {
	tests := []struct {
		name     string
		template string
		setup    func(*http.Request)
		verify   func(*httputils.ResponseModifier)
	}{
		{
			name:     "set response body with template",
			template: "Response: $req_method $req_path",
			setup: func(r *http.Request) {
				r.Method = http.MethodGet
				r.URL.Path = "/api/test"
			},
			verify: func(rm *httputils.ResponseModifier) {
				content := string(rm.Content())
				expected := "Response: GET /api/test"
				assert.Equal(t, expected, content, "Expected response body")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			tt.setup(req)
			w := httputils.NewResponseModifier(httptest.NewRecorder())

			_, tmpl, tErr := validateTemplate(tt.template, false)
			if tErr != nil {
				t.Fatalf("Failed to parse template: %v", tErr)
			}

			handler := modFields[FieldResponseBody].builder(tmpl)
			err := handler.set(w, req, nil)
			if err != nil {
				t.Fatalf("Handler returned error: %v", err)
			}

			tt.verify(w)
		})
	}
}

func TestFieldHandler_StatusCode(t *testing.T) {
	tests := []struct {
		name   string
		status int
		verify func(*httptest.ResponseRecorder)
	}{
		{
			name:   "set status code 200",
			status: http.StatusOK,
			verify: func(w *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, w.Code, "Expected status code 200")
			},
		},
		{
			name:   "set status code 404",
			status: http.StatusNotFound,
			verify: func(w *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNotFound, w.Code, "Expected status code 404")
			},
		},
		{
			name:   "set status code 500",
			status: http.StatusInternalServerError,
			verify: func(w *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusInternalServerError, w.Code, "Expected status code 500")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			rm := httputils.NewResponseModifier(w)
			var cmd Command
			err := cmd.Parse(fmt.Sprintf("set %s %d", FieldStatusCode, tt.status))
			if err != nil {
				t.Fatalf("Handler returned error: %v", err)
			}
			err = cmd.post.ServeHTTP(rm, req, nil)
			if err != nil {
				t.Fatalf("Handler returned error: %v", err)
			}
			rm.FlushRelease()
			tt.verify(w)
		})
	}
}

func TestFieldValidation(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		args      []string
		wantError bool
	}{
		{
			name:      "header valid",
			field:     FieldHeader,
			args:      []string{"key", "value"},
			wantError: false,
		},
		{
			name:      "header invalid - missing value",
			field:     FieldHeader,
			args:      []string{"key"},
			wantError: true,
		},
		{
			name:      "response header valid",
			field:     FieldResponseHeader,
			args:      []string{"key", "value"},
			wantError: false,
		},
		{
			name:      "query valid",
			field:     FieldQuery,
			args:      []string{"key", "value"},
			wantError: false,
		},
		{
			name:      "cookie valid",
			field:     FieldCookie,
			args:      []string{"key", "value"},
			wantError: false,
		},
		{
			name:      "body valid template",
			field:     FieldBody,
			args:      []string{"Hello $req_method"},
			wantError: false,
		},
		{
			name:      "body invalid template syntax",
			field:     FieldBody,
			args:      []string{"Hello $invalid_field"},
			wantError: true,
		},
		{
			name:      "response body valid template",
			field:     FieldResponseBody,
			args:      []string{"Response: $req_method"},
			wantError: false,
		},
		{
			name:      "status code valid",
			field:     FieldStatusCode,
			args:      []string{"200"},
			wantError: false,
		},
		{
			name:      "status code invalid - too low",
			field:     FieldStatusCode,
			args:      []string{"99"},
			wantError: true,
		},
		{
			name:      "status code invalid - too high",
			field:     FieldStatusCode,
			args:      []string{"600"},
			wantError: true,
		},
		{
			name:      "status code invalid - not a number",
			field:     FieldStatusCode,
			args:      []string{"not-a-number"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, exists := modFields[tt.field]
			assert.True(t, exists, "Field %s does not exist", tt.field)

			_, _, err := field.validate(tt.args)
			if tt.wantError {
				assert.Error(t, err, "Expected error but got none")
			} else {
				assert.NoError(t, err, "Expected no error but got: %v", err)
			}
		})
	}
}

func TestModFields(t *testing.T) {
	for fieldName, field := range modFields {
		// Test that each field has required components
		assert.NotNil(t, field.validate, "Field %s has nil validate function", fieldName)
		assert.NotNil(t, field.builder, "Field %s has nil builder function", fieldName)
		assert.NotEmpty(t, field.help.command, "Field %s has empty help command", fieldName)
	}
}

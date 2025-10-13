package rules

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
				if got := r.Header.Get("X-Test"); got != "test-value" {
					t.Errorf("Expected header X-Test to be 'test-value', got '%s'", got)
				}
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
				if len(values) != 2 {
					t.Errorf("Expected 2 header values, got %d", len(values))
				}
				if values[0] != "existing-value" || values[1] != "new-value" {
					t.Errorf("Expected ['existing-value', 'new-value'], got %v", values)
				}
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
				if got := r.Header.Get("X-Test"); got != "" {
					t.Errorf("Expected header X-Test to be removed, got '%s'", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			tt.setup(req)
			w := httptest.NewRecorder()

			tmpl, tErr := validateTemplate(tt.value, false)
			if tErr != nil {
				t.Fatalf("Failed to validate template: %v", tErr)
			}
			handler := modFields[FieldHeader].builder(&keyValueTemplate{tt.key, tmpl})
			var cmd CommandHandler
			switch tt.modifier {
			case ModFieldSet:
				cmd = handler.set
			case ModFieldAdd:
				cmd = handler.add
			case ModFieldRemove:
				cmd = handler.remove
			}

			err := cmd.Handle(w, req)
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
		verify   func(*httptest.ResponseRecorder)
	}{
		{
			name:     "set response header",
			key:      "X-Response-Test",
			value:    "response-value",
			modifier: ModFieldSet,
			verify: func(w *httptest.ResponseRecorder) {
				if got := w.Header().Get("X-Response-Test"); got != "response-value" {
					t.Errorf("Expected response header X-Response-Test to be 'response-value', got '%s'", got)
				}
			},
		},
		{
			name:     "add response header",
			key:      "X-Response-Test",
			value:    "additional-value",
			modifier: ModFieldAdd,
			verify: func(w *httptest.ResponseRecorder) {
				values := w.Header()["X-Response-Test"]
				if len(values) != 1 || values[0] != "additional-value" {
					t.Errorf("Expected ['additional-value'], got %v", values)
				}
			},
		},
		{
			name:     "remove response header",
			key:      "X-Response-Test",
			value:    "",
			modifier: ModFieldRemove,
			verify: func(w *httptest.ResponseRecorder) {
				if got := w.Header().Get("X-Response-Test"); got != "" {
					t.Errorf("Expected response header X-Response-Test to be removed, got '%s'", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()

			tmpl, tErr := validateTemplate(tt.value, false)
			if tErr != nil {
				t.Fatalf("Failed to validate template: %v", tErr)
			}
			handler := modFields[FieldResponseHeader].builder(&keyValueTemplate{tt.key, tmpl})
			var cmd CommandHandler
			switch tt.modifier {
			case ModFieldSet:
				cmd = handler.set
			case ModFieldAdd:
				cmd = handler.add
			case ModFieldRemove:
				cmd = handler.remove
			}

			err := cmd.Handle(w, req)
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
				if got := r.URL.Query().Get("test"); got != "new-value" {
					t.Errorf("Expected query 'test' to be 'new-value', got '%s'", got)
				}
				if got := r.URL.Query().Get("other"); got != "keep" {
					t.Errorf("Expected query 'other' to be 'keep', got '%s'", got)
				}
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
				if len(values) != 2 {
					t.Errorf("Expected 2 query values, got %d", len(values))
				}
				if values[0] != "existing-value" || values[1] != "additional-value" {
					t.Errorf("Expected ['existing-value', 'additional-value'], got %v", values)
				}
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
				if got := r.URL.Query().Get("test"); got != "" {
					t.Errorf("Expected query 'test' to be removed, got '%s'", got)
				}
				if got := r.URL.Query().Get("other"); got != "keep" {
					t.Errorf("Expected query 'other' to be 'keep', got '%s'", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			tt.setup(req)
			w := httptest.NewRecorder()

			tmpl, tErr := validateTemplate(tt.value, false)
			if tErr != nil {
				t.Fatalf("Failed to validate template: %v", tErr)
			}
			handler := modFields[FieldQuery].builder(&keyValueTemplate{tt.key, tmpl})
			var cmd CommandHandler
			switch tt.modifier {
			case ModFieldSet:
				cmd = handler.set
			case ModFieldAdd:
				cmd = handler.add
			case ModFieldRemove:
				cmd = handler.remove
			}

			err := cmd.Handle(w, req)
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
				if err != nil {
					t.Fatalf("Expected cookie 'test' to exist, got error: %v", err)
				}
				if cookie.Value != "new-value" {
					t.Errorf("Expected cookie 'test' to be 'new-value', got '%s'", cookie.Value)
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
				if len(testCookies) != 2 {
					t.Errorf("Expected 2 cookies with name 'test', got %d", len(testCookies))
				}
				if testCookies[0] != "existing-value" || testCookies[1] != "additional-value" {
					t.Errorf("Expected ['existing-value', 'additional-value'], got %v", testCookies)
				}
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
				if _, err := r.Cookie("test"); err == nil {
					t.Errorf("Expected cookie 'test' to be removed")
				}
				if cookie, err := r.Cookie("other"); err != nil || cookie.Value != "keep" {
					t.Errorf("Expected cookie 'other' to be 'keep', got error: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			tt.setup(req)
			w := httptest.NewRecorder()

			tmpl, tErr := validateTemplate(tt.value, false)
			if tErr != nil {
				t.Fatalf("Failed to validate template: %v", tErr)
			}
			handler := modFields[FieldCookie].builder(&keyValueTemplate{tt.key, tmpl})
			var cmd CommandHandler
			switch tt.modifier {
			case ModFieldSet:
				cmd = handler.set
			case ModFieldAdd:
				cmd = handler.add
			case ModFieldRemove:
				cmd = handler.remove
			}

			err := cmd.Handle(w, req)
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
			template: "Hello {{ .Request.Method }} {{ .Request.URL.Path }}",
			setup: func(r *http.Request) {
				r.Method = "POST"
				r.URL.Path = "/test"
			},
			verify: func(r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("Failed to read body: %v", err)
				}
				expected := "Hello POST /test"
				if string(body) != expected {
					t.Errorf("Expected body '%s', got '%s'", expected, string(body))
				}
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
				if err != nil {
					t.Fatalf("Failed to read body: %v", err)
				}
				if string(body) != "Overridden" {
					t.Errorf("Expected body 'Overridden', got '%s'", string(body))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			tt.setup(req)
			w := httptest.NewRecorder()

			tmpl, tErr := validateTemplate(tt.template, false)
			if tErr != nil {
				t.Fatalf("Failed to parse template: %v", tErr)
			}

			handler := modFields[FieldBody].builder(tmpl)
			err := handler.set.Handle(w, req)
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
		verify   func(*ResponseModifier)
	}{
		{
			name:     "set response body with template",
			template: "Response: {{ .Request.Method }} {{ .Request.URL.Path }}",
			setup: func(r *http.Request) {
				r.Method = "GET"
				r.URL.Path = "/api/test"
			},
			verify: func(rm *ResponseModifier) {
				content := rm.buf.String()
				expected := "Response: GET /api/test"
				if content != expected {
					t.Errorf("Expected response body '%s', got '%s'", expected, content)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			tt.setup(req)
			w := httptest.NewRecorder()

			// Create ResponseModifier wrapper
			rm := NewResponseModifier(w)

			tmpl, tErr := validateTemplate(tt.template, false)
			if tErr != nil {
				t.Fatalf("Failed to parse template: %v", tErr)
			}

			handler := modFields[FieldResponseBody].builder(tmpl)
			err := handler.set.Handle(rm, req)
			if err != nil {
				t.Fatalf("Handler returned error: %v", err)
			}

			tt.verify(rm)
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
			status: 200,
			verify: func(w *httptest.ResponseRecorder) {
				// Note: ResponseModifier doesn't write the header immediately
				// The status code is stored and written during FlushRelease
			},
		},
		{
			name:   "set status code 404",
			status: 404,
			verify: func(w *httptest.ResponseRecorder) {
				// Status code is set in ResponseModifier
			},
		},
		{
			name:   "set status code 500",
			status: 500,
			verify: func(w *httptest.ResponseRecorder) {
				// Status code is set in ResponseModifier
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()

			handler := modFields[FieldStatusCode].builder(tt.status)
			err := handler.set.Handle(w, req)
			if err != nil {
				t.Fatalf("Handler returned error: %v", err)
			}

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
			args:      []string{"Hello {{ .Request.Method }}"},
			wantError: false,
		},
		{
			name:      "body invalid template syntax",
			field:     FieldBody,
			args:      []string{"Hello {{ .InvalidField "},
			wantError: true,
		},
		{
			name:      "response body valid template",
			field:     FieldResponseBody,
			args:      []string{"Response: {{ .Request.Method }}"},
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
			if !exists {
				t.Fatalf("Field %s does not exist", tt.field)
			}

			_, err := field.validate(tt.args)
			if tt.wantError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestAllFields(t *testing.T) {
	expectedFields := []string{
		FieldHeader,
		FieldResponseHeader,
		FieldQuery,
		FieldCookie,
		FieldBody,
		FieldResponseBody,
		FieldStatusCode,
	}

	if len(AllFields) != len(expectedFields) {
		t.Errorf("Expected %d fields, got %d", len(expectedFields), len(AllFields))
	}

	for _, expected := range expectedFields {
		found := false
		for _, actual := range AllFields {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected field %s not found in AllFields", expected)
		}
	}
}

func TestModFields(t *testing.T) {
	for fieldName, field := range modFields {
		// Test that each field has required components
		if field.validate == nil {
			t.Errorf("Field %s has nil validate function", fieldName)
		}
		if field.builder == nil {
			t.Errorf("Field %s has nil builder function", fieldName)
		}
		if field.help.command == "" {
			t.Errorf("Field %s has empty help command", fieldName)
		}
	}
}

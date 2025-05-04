package httpheaders

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppendCSP(t *testing.T) {
	tests := []struct {
		name           string
		initialHeaders map[string][]string
		sources        []string
		directives     []string
		expectedCSP    map[string]string
	}{
		{
			name:           "No CSP header",
			initialHeaders: map[string][]string{},
			sources:        []string{},
			directives:     []string{"default-src", "script-src", "frame-src", "style-src", "connect-src"},
			expectedCSP:    map[string]string{"default-src": "'self'", "script-src": "'self'", "frame-src": "'self'", "style-src": "'self'", "connect-src": "'self'"},
		},
		{
			name:           "No CSP header with sources",
			initialHeaders: map[string][]string{},
			sources:        []string{"https://example.com"},
			directives:     []string{"default-src", "script-src", "frame-src", "style-src", "connect-src"},
			expectedCSP:    map[string]string{"default-src": "'self' https://example.com", "script-src": "'self' https://example.com", "frame-src": "'self' https://example.com", "style-src": "'self' https://example.com", "connect-src": "'self' https://example.com"},
		},
		{
			name: "replace 'none' with sources",
			initialHeaders: map[string][]string{
				"Content-Security-Policy": {"default-src 'none'"},
			},
			sources:     []string{"https://example.com"},
			directives:  []string{"default-src"},
			expectedCSP: map[string]string{"default-src": "https://example.com"},
		},
		{
			name: "CSP header with some directives",
			initialHeaders: map[string][]string{
				"Content-Security-Policy": {"default-src 'none'", "script-src 'unsafe-inline'"},
			},
			sources:    []string{"https://example.com"},
			directives: []string{"script-src"},
			expectedCSP: map[string]string{
				"default-src": "'none",
				"script-src":  "'unsafe-inline' https://example.com",
			},
		},
		{
			name: "CSP header with some directives with self",
			initialHeaders: map[string][]string{
				"Content-Security-Policy": {"default-src 'self'", "connect-src 'self'"},
			},
			sources:    []string{"https://api.example.com"},
			directives: []string{"default-src", "connect-src"},
			expectedCSP: map[string]string{
				"default-src": "'self' https://api.example.com",
				"connect-src": "'self' https://api.example.com",
			},
		},
		{
			name: "AppendCSP sources conflict with existing CSP header",
			initialHeaders: map[string][]string{
				"Content-Security-Policy": {"default-src 'self' https://cdn.example.com", "script-src 'unsafe-inline'"},
			},
			sources:    []string{"https://cdn.example.com", "https://api.example.com"},
			directives: []string{"default-src", "script-src"},
			expectedCSP: map[string]string{
				"default-src": "'self' https://cdn.example.com https://api.example.com",
				"script-src":  "'unsafe-inline' https://cdn.example.com https://api.example.com",
			},
		},
		{
			name: "Non-standard CSP directive",
			initialHeaders: map[string][]string{
				"Content-Security-Policy": {
					"default-src 'self'",
					"script-src 'unsafe-inline'",
					"img-src 'self'", // img-src is not in cspDirectives list
				},
			},
			sources:    []string{"https://example.com"},
			directives: []string{"default-src", "script-src"},
			expectedCSP: map[string]string{
				"default-src": "'self' https://example.com",
				"script-src":  "'unsafe-inline' https://example.com",
				// img-src should not be present in response as it's not in cspDirectives
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test request with initial headers
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for header, values := range tc.initialHeaders {
				req.Header[header] = values
			}

			// Create a test response recorder
			w := httptest.NewRecorder()

			// Call the function under test
			AppendCSP(w, req, tc.directives, tc.sources)

			// Check the resulting CSP headers
			respHeaders := w.Header()
			cspValues, exists := respHeaders["Content-Security-Policy"]

			// If we expect no CSP headers, verify none exist
			if len(tc.expectedCSP) == 0 {
				if exists && len(cspValues) > 0 {
					t.Errorf("Expected no CSP header, but got %v", cspValues)
				}
				return
			}

			// Verify CSP headers exist when expected
			if !exists || len(cspValues) == 0 {
				t.Errorf("Expected CSP header to be set, but it was not")
				return
			}

			// Parse the CSP response and verify each directive
			foundDirectives := make(map[string]string)
			for _, cspValue := range cspValues {
				parts := strings.Split(cspValue, ";")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}

					directiveParts := strings.SplitN(part, " ", 2)
					if len(directiveParts) != 2 {
						t.Errorf("Invalid CSP directive format: %s", part)
						continue
					}

					directive := directiveParts[0]
					value := directiveParts[1]
					foundDirectives[directive] = value
				}
			}

			// Verify expected directives
			for directive, expectedValue := range tc.expectedCSP {
				actualValue, ok := foundDirectives[directive]
				if !ok {
					t.Errorf("Expected directive %s not found in response", directive)
					continue
				}

				// Check if all expected sources are in the actual value
				expectedSources := strings.SplitSeq(expectedValue, " ")
				for source := range expectedSources {
					if !strings.Contains(actualValue, source) {
						t.Errorf("Directive %s missing expected source %s. Got: %s", directive, source, actualValue)
					}
				}
			}
		})
	}
}

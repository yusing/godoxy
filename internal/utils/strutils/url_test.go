package strutils

import (
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestSanitizeURI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "/",
		},
		{
			name:     "single slash",
			input:    "/",
			expected: "/",
		},
		{
			name:     "normal path",
			input:    "/path/to/resource",
			expected: "/path/to/resource",
		},
		{
			name:     "path without leading slash",
			input:    "path/to/resource",
			expected: "/path/to/resource",
		},
		{
			name:     "path with dot segments",
			input:    "/path/./to/../resource",
			expected: "/path/resource",
		},
		{
			name:     "double slash prefix",
			input:    "//path/to/resource",
			expected: "/",
		},
		{
			name:     "backslash prefix",
			input:    "/\\path/to/resource",
			expected: "/",
		},
		{
			name:     "path with multiple slashes",
			input:    "/path//to///resource",
			expected: "/path/to/resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeURI(tt.input)
			expect.Equal(t, result, tt.expected)
		})
	}
}

package strutils

import (
	"testing"

	"github.com/stretchr/testify/require"
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
			require.Equal(t, tt.expected, result)
		})
	}
}

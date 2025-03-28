package accesslog

import (
	"fmt"
	"strings"
	"testing"
)

func TestBackScanner(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty file",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single line without newline",
			input:    "single line",
			expected: []string{"single line"},
		},
		{
			name:     "single line with newline",
			input:    "single line\n",
			expected: []string{"single line"},
		},
		{
			name:     "multiple lines",
			input:    "first\nsecond\nthird\n",
			expected: []string{"third", "second", "first"},
		},
		{
			name:     "multiple lines without final newline",
			input:    "first\nsecond\nthird",
			expected: []string{"third", "second", "first"},
		},
		{
			name:     "lines longer than chunk size",
			input:    "short\n" + strings.Repeat("a", 20) + "\nshort\n",
			expected: []string{"short", strings.Repeat("a", 20), "short"},
		},
		{
			name:     "empty lines",
			input:    "first\n\n\nlast\n",
			expected: []string{"last", "first"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock file
			mockFile := &MockFile{}
			_, err := mockFile.Write([]byte(tt.input))
			if err != nil {
				t.Fatalf("failed to write to mock file: %v", err)
			}

			// Create scanner with small chunk size to test chunking
			scanner := NewBackScanner(mockFile, 10)

			// Collect all lines
			var lines [][]byte
			for scanner.Scan() {
				lines = append(lines, scanner.Bytes())
			}

			// Check for scanning errors
			if err := scanner.Err(); err != nil {
				t.Errorf("scanner error: %v", err)
			}

			// Compare results
			if len(lines) != len(tt.expected) {
				t.Errorf("got %d lines, want %d lines", len(lines), len(tt.expected))
				return
			}

			for i, line := range lines {
				if string(line) != tt.expected[i] {
					t.Errorf("line %d: got %q, want %q", i, line, tt.expected[i])
				}
			}
		})
	}
}

func TestBackScannerWithVaryingChunkSizes(t *testing.T) {
	input := "first\nsecond\nthird\nfourth\nfifth\n"
	expected := []string{"fifth", "fourth", "third", "second", "first"}
	chunkSizes := []int{1, 2, 3, 5, 10, 20, 100}

	for _, chunkSize := range chunkSizes {
		t.Run(fmt.Sprintf("chunk_size_%d", chunkSize), func(t *testing.T) {
			mockFile := &MockFile{}
			_, err := mockFile.Write([]byte(input))
			if err != nil {
				t.Fatalf("failed to write to mock file: %v", err)
			}

			scanner := NewBackScanner(mockFile, chunkSize)

			var lines [][]byte
			for scanner.Scan() {
				lines = append(lines, scanner.Bytes())
			}

			if err := scanner.Err(); err != nil {
				t.Errorf("scanner error: %v", err)
			}

			if len(lines) != len(expected) {
				t.Errorf("got %d lines, want %d lines", len(lines), len(expected))
				return
			}

			for i, line := range lines {
				if string(line) != expected[i] {
					t.Errorf("chunk size %d, line %d: got %q, want %q",
						chunkSize, i, line, expected[i])
				}
			}
		})
	}
}

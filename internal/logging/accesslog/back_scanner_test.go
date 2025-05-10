package accesslog

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
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
			mockFile := NewMockFile()
			_, err := mockFile.Write([]byte(tt.input))
			if err != nil {
				t.Fatalf("failed to write to mock file: %v", err)
			}

			// Create scanner with small chunk size to test chunking
			scanner := NewBackScanner(mockFile, mockFile.MustSize(), 10)

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
			mockFile := NewMockFile()
			_, err := mockFile.Write([]byte(input))
			if err != nil {
				t.Fatalf("failed to write to mock file: %v", err)
			}

			scanner := NewBackScanner(mockFile, mockFile.MustSize(), chunkSize)

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

func logEntry() []byte {
	accesslog := NewMockAccessLogger(task.RootTask("test", false), &RequestLoggerConfig{
		Format: FormatJSON,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	srv.URL = "http://localhost:8080"
	defer srv.Close()
	// make a request to the server
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	res := httptest.NewRecorder()
	// server the request
	srv.Config.Handler.ServeHTTP(res, req)
	b := accesslog.AppendRequestLog(nil, req, res.Result())
	if b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return b
}

func TestReset(t *testing.T) {
	file, err := afero.TempFile(afero.NewOsFs(), "", "accesslog")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())
	line := logEntry()
	nLines := 1000
	for range nLines {
		_, err := file.Write(line)
		if err != nil {
			t.Fatalf("failed to write to temp file: %v", err)
		}
	}
	linesRead := 0
	stat, _ := file.Stat()
	s := NewBackScanner(file, stat.Size(), defaultChunkSize)
	for s.Scan() {
		linesRead++
	}
	if err := s.Err(); err != nil {
		t.Errorf("scanner error: %v", err)
	}
	expect.Equal(t, linesRead, nLines)
	err = s.Reset()
	if err != nil {
		t.Errorf("failed to reset scanner: %v", err)
	}

	linesRead = 0
	for s.Scan() {
		linesRead++
	}
	if err := s.Err(); err != nil {
		t.Errorf("scanner error: %v", err)
	}
	expect.Equal(t, linesRead, nLines)
}

// 100000 log entries.
func BenchmarkBackScanner(b *testing.B) {
	mockFile := NewMockFile()
	line := logEntry()
	for range 100000 {
		_, _ = mockFile.Write(line)
	}
	for i := range 14 {
		chunkSize := (2 << i) * kilobyte
		scanner := NewBackScanner(mockFile, mockFile.MustSize(), chunkSize)
		name := strutils.FormatByteSize(chunkSize)
		b.ResetTimer()
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				_ = scanner.Reset()
				for scanner.Scan() {
				}
			}
		})
	}
}

func BenchmarkBackScannerRealFile(b *testing.B) {
	file, err := afero.TempFile(afero.NewOsFs(), "", "accesslog")
	if err != nil {
		b.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())

	for range 10000 {
		_, err = file.Write(logEntry())
		if err != nil {
			b.Fatalf("failed to write to temp file: %v", err)
		}
	}

	stat, _ := file.Stat()
	scanner := NewBackScanner(file, stat.Size(), 256*kilobyte)
	b.ResetTimer()
	for scanner.Scan() {
	}
	if err := scanner.Err(); err != nil {
		b.Errorf("scanner error: %v", err)
	}
}

/*
BenchmarkBackScanner
BenchmarkBackScanner/2_KiB
BenchmarkBackScanner/2_KiB-20         	      52	  23254071 ns/op	67596663 B/op	   26420 allocs/op
BenchmarkBackScanner/4_KiB
BenchmarkBackScanner/4_KiB-20         	      55	  20961059 ns/op	62529378 B/op	   13211 allocs/op
BenchmarkBackScanner/8_KiB
BenchmarkBackScanner/8_KiB-20         	      64	  18242460 ns/op	62951141 B/op	    6608 allocs/op
BenchmarkBackScanner/16_KiB
BenchmarkBackScanner/16_KiB-20        	      52	  20162076 ns/op	62940256 B/op	    3306 allocs/op
BenchmarkBackScanner/32_KiB
BenchmarkBackScanner/32_KiB-20        	      54	  19247968 ns/op	67553645 B/op	    1656 allocs/op
BenchmarkBackScanner/64_KiB
BenchmarkBackScanner/64_KiB-20        	      60	  20909046 ns/op	64053342 B/op	     827 allocs/op
BenchmarkBackScanner/128_KiB
BenchmarkBackScanner/128_KiB-20       	      68	  17759890 ns/op	62201945 B/op	     414 allocs/op
BenchmarkBackScanner/256_KiB
BenchmarkBackScanner/256_KiB-20       	      52	  19531877 ns/op	61030487 B/op	     208 allocs/op
BenchmarkBackScanner/512_KiB
BenchmarkBackScanner/512_KiB-20       	      54	  19124656 ns/op	61030485 B/op	     208 allocs/op
BenchmarkBackScanner/1_MiB
BenchmarkBackScanner/1_MiB-20         	      67	  17078936 ns/op	61030495 B/op	     208 allocs/op
BenchmarkBackScanner/2_MiB
BenchmarkBackScanner/2_MiB-20         	      66	  18467421 ns/op	61030492 B/op	     208 allocs/op
BenchmarkBackScanner/4_MiB
BenchmarkBackScanner/4_MiB-20         	      68	  17214573 ns/op	61030486 B/op	     208 allocs/op
BenchmarkBackScanner/8_MiB
BenchmarkBackScanner/8_MiB-20         	      57	  18235229 ns/op	61030492 B/op	     208 allocs/op
BenchmarkBackScanner/16_MiB
BenchmarkBackScanner/16_MiB-20        	      57	  19343441 ns/op	61030499 B/op	     208 allocs/op
*/

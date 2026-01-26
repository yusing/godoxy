package accesslog

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/afero"

	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
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
			mockFile := NewMockFile(false)
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
			mockFile := NewMockFile(false)
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

var logEntry = func() func() []byte {
	accesslog := NewMockAccessLogger(task.RootTask("test", false), &RequestLoggerConfig{
		Format: FormatJSON,
	})

	contentTypes := []string{"application/json", "text/html", "text/plain", "application/xml", "application/x-www-form-urlencoded"}
	userAgents := []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Firefox/120.0", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Firefox/120.0", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Firefox/120.0"}
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"}
	paths := []string{"/", "/about", "/contact", "/login", "/logout", "/register", "/profile"}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allocSize := rand.IntN(8192)
		w.Header().Set("Content-Type", contentTypes[rand.IntN(len(contentTypes))])
		w.Header().Set("Content-Length", strconv.Itoa(allocSize))
		w.WriteHeader(http.StatusOK)
	}))
	srv.URL = "http://localhost:8080"

	return func() []byte {
		// make a request to the server
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		res := httptest.NewRecorder()
		req.Header.Set("User-Agent", userAgents[rand.IntN(len(userAgents))])
		req.Method = methods[rand.IntN(len(methods))]
		req.URL.Path = paths[rand.IntN(len(paths))]
		// server the request
		srv.Config.Handler.ServeHTTP(res, req)
		b := bytes.NewBuffer(make([]byte, 0, 1024))
		accesslog.(RequestFormatter).AppendRequestLog(b, req, res.Result())
		return b.Bytes()
	}
}()

// 100000 log entries.
func BenchmarkBackScannerRealFile(b *testing.B) {
	file, err := afero.TempFile(afero.NewOsFs(), "", "accesslog")
	if err != nil {
		b.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())

	buf := bytes.NewBuffer(nil)
	for range 100000 {
		buf.Write(logEntry())
	}

	fSize := int64(buf.Len())
	_, err = file.Write(buf.Bytes())
	if err != nil {
		b.Fatalf("failed to write to file: %v", err)
	}

	// file position does not matter, Seek not needed

	for i := range 12 {
		chunkSize := (2 << i) * kilobyte
		name := strutils.FormatByteSize(chunkSize)
		b.ResetTimer()
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				scanner := NewBackScanner(file, fSize, chunkSize)
				for scanner.Scan() {
				}
				scanner.Release()
			}
		})
	}
}

/*
BenchmarkBackScannerRealFile
BenchmarkBackScannerRealFile/2_KiB
BenchmarkBackScannerRealFile/2_KiB-10                 21          51796773 ns/op             619 B/op          1 allocs/op
BenchmarkBackScannerRealFile/4_KiB
BenchmarkBackScannerRealFile/4_KiB-10                 36          32081281 ns/op             699 B/op          1 allocs/op
BenchmarkBackScannerRealFile/8_KiB
BenchmarkBackScannerRealFile/8_KiB-10                 57          22155619 ns/op             847 B/op          1 allocs/op
BenchmarkBackScannerRealFile/16_KiB
BenchmarkBackScannerRealFile/16_KiB-10                62          21323125 ns/op            1449 B/op          1 allocs/op
BenchmarkBackScannerRealFile/32_KiB
BenchmarkBackScannerRealFile/32_KiB-10                63          17534883 ns/op            2729 B/op          1 allocs/op
BenchmarkBackScannerRealFile/64_KiB
BenchmarkBackScannerRealFile/64_KiB-10                73          17877029 ns/op            4617 B/op          1 allocs/op
BenchmarkBackScannerRealFile/128_KiB
BenchmarkBackScannerRealFile/128_KiB-10               75          17797267 ns/op            8866 B/op          1 allocs/op
BenchmarkBackScannerRealFile/256_KiB
BenchmarkBackScannerRealFile/256_KiB-10               67          16732108 ns/op           19691 B/op          1 allocs/op
BenchmarkBackScannerRealFile/512_KiB
BenchmarkBackScannerRealFile/512_KiB-10               70          17121683 ns/op           37577 B/op          1 allocs/op
BenchmarkBackScannerRealFile/1_MiB
BenchmarkBackScannerRealFile/1_MiB-10                 51          19615791 ns/op          102930 B/op          1 allocs/op
BenchmarkBackScannerRealFile/2_MiB
BenchmarkBackScannerRealFile/2_MiB-10                 26          41744928 ns/op        77595287 B/op         57 allocs/op
BenchmarkBackScannerRealFile/4_MiB
BenchmarkBackScannerRealFile/4_MiB-10                 22          48081521 ns/op        79692224 B/op         49 allocs/op
*/

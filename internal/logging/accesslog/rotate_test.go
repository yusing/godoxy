package accesslog_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/goutils/mockable"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
	expect "github.com/yusing/goutils/testing"
)

var (
	testTime    = expect.Must(time.Parse(time.RFC3339, "2024-01-31T03:04:05Z"))
	testTimeStr = testTime.Format(LogTimeFormat)
)

func TestParseLogTime(t *testing.T) {
	t.Run("valid time", func(t *testing.T) {
		tests := []string{
			`{"foo":"bar","time":"%s","bar":"baz"}`,
			`example.com 192.168.1.1 - - [%s] "GET / HTTP/1.1" 200 1234`,
		}

		for i, test := range tests {
			tests[i] = fmt.Sprintf(test, testTimeStr)
		}

		for _, test := range tests {
			t.Run(test, func(t *testing.T) {
				extracted := ExtractTime([]byte(test))
				expect.Equal(t, string(extracted), testTimeStr)
				got := ParseLogTime([]byte(test))
				expect.True(t, got.Equal(testTime), "expected %s, got %s", testTime, got)
			})
		}
	})

	t.Run("invalid time", func(t *testing.T) {
		tests := []string{
			`{"foo":"bar","time":"invalid","bar":"baz"}`,
			`example.com 192.168.1.1 - - [invalid] "GET / HTTP/1.1" 200 1234`,
		}
		for _, test := range tests {
			t.Run(test, func(t *testing.T) {
				expect.True(t, ParseLogTime([]byte(test)).IsZero(), "expected zero time, got %s", ParseLogTime([]byte(test)))
			})
		}
	})
}

func TestRotateKeepLast(t *testing.T) {
	for _, format := range ReqLoggerFormats {
		t.Run(string(format)+" keep last", func(t *testing.T) {
			file := newMockFile(true)
			mockable.MockTimeNow(testTime)
			logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
				Format: format,
			})
			expect.Nil(t, logger.Config().Retention)

			for range 10 {
				logger.LogRequest(req, resp)
			}
			logger.Flush()

			expect.Greater(t, file.Len(), int64(0))
			expect.Equal(t, file.NumLines(), 10)

			retention := strutils.MustParse[*Retention]("last 5")
			expect.Equal(t, retention.Days, 0)
			expect.Equal(t, retention.Last, 5)
			expect.Equal(t, retention.KeepSize, 0)
			logger.Config().Retention = retention

			var result RotateResult
			rotated, err := logger.(AccessLogRotater).Rotate(&result)
			expect.NoError(t, err)
			expect.Equal(t, rotated, true)
			expect.Equal(t, file.NumLines(), int(retention.Last))
			expect.Equal(t, result.NumLinesKeep, int(retention.Last))
			expect.Equal(t, result.NumLinesInvalid, 0)
		})

		t.Run(string(format)+" keep days", func(t *testing.T) {
			file := newMockFile(true)
			logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
				Format: format,
			})
			expect.Nil(t, logger.Config().Retention)
			nLines := 10
			for i := range nLines {
				mockable.MockTimeNow(testTime.AddDate(0, 0, -nLines+i+1))
				logger.LogRequest(req, resp)
			}
			logger.Flush()
			expect.Equal(t, file.NumLines(), nLines)

			retention := strutils.MustParse[*Retention]("3 days")
			expect.Equal(t, retention.Days, 3)
			expect.Equal(t, retention.Last, 0)
			expect.Equal(t, retention.KeepSize, 0)
			logger.Config().Retention = retention

			mockable.MockTimeNow(testTime)
			var result RotateResult
			rotated, err := logger.(AccessLogRotater).Rotate(&result)
			expect.NoError(t, err)
			expect.Equal(t, rotated, true)
			expect.Equal(t, file.NumLines(), int(retention.Days))
			expect.Equal(t, result.NumLinesKeep, int(retention.Days))
			expect.Equal(t, result.NumLinesInvalid, 0)

			rotatedLines := bytes.Split(file.Content(), []byte("\n"))
			for i, line := range rotatedLines {
				if i >= int(retention.Days) { // may ends with a newline
					break
				}
				timeBytes := ExtractTime(line)
				got, err := time.Parse(LogTimeFormat, string(timeBytes))
				expect.NoError(t, err)
				want := testTime.AddDate(0, 0, -int(retention.Days)+i+1)
				expect.True(t, got.Equal(want), "expected %s, got %s", want, got)
			}
		})
	}
}

func TestRotateKeepDaysSkipsFreshLogScan(t *testing.T) {
	file := newMockFile(true)
	logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
		Format: FormatJSON,
	})
	for range 1000 {
		mockable.MockTimeNow(testTime)
		logger.LogRequest(req, resp)
	}
	logger.Flush()
	content := file.Content()

	logger.Config().Retention = strutils.MustParse[*Retention]("30 days")
	mockable.MockTimeNow(testTime)
	var result RotateResult
	rotated, err := logger.(AccessLogRotater).Rotate(&result)

	expect.NoError(t, err)
	expect.Equal(t, rotated, false)
	expect.Equal(t, file.Content(), content)
	expect.Equal(t, result.NumLinesRead, 0)
	expect.Equal(t, result.NumBytesKeep, file.Len())
}

func TestRotateKeepDaysRollsRealFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	file, err := OpenFile(path)
	expect.NoError(t, err)

	logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
		Format: FormatJSON,
		ConfigBase: ConfigBase{
			Retention: strutils.MustParse[*Retention]("30 days"),
		},
	})
	defer logger.Close()

	for range 3 {
		mockable.MockTimeNow(testTime)
		logger.LogRequest(req, resp)
	}
	logger.Flush()
	content := expect.Must(os.ReadFile(path))

	var result RotateResult
	rotated, err := logger.(AccessLogRotater).Rotate(&result)

	expect.NoError(t, err)
	expect.True(t, rotated)
	expect.Equal(t, result.NumLinesRead, 0)
	expect.Equal(t, result.NumBytesKeep, int64(len(content)))

	active, err := os.ReadFile(path)
	expect.NoError(t, err)
	expect.Equal(t, len(active), 0)

	archives := expect.Must(filepath.Glob(path + ".*"))
	expect.Equal(t, len(archives), 1)
	archived := expect.Must(os.ReadFile(archives[0]))
	expect.Equal(t, archived, content)

	logger.LogRequest(req, resp)
	logger.Flush()
	active, err = os.ReadFile(path)
	expect.NoError(t, err)
	expect.Greater(t, int64(len(active)), int64(0))
}

func TestRotateKeepDaysPreservesActiveFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	expect.NoError(t, os.WriteFile(path, nil, 0o644))
	expect.NoError(t, os.Chmod(path, 0o660))

	file, err := OpenFile(path)
	expect.NoError(t, err)

	logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
		Format: FormatJSON,
		ConfigBase: ConfigBase{
			Retention: strutils.MustParse[*Retention]("30 days"),
		},
	})
	defer logger.Close()

	mockable.MockTimeNow(testTime)
	logger.LogRequest(req, resp)

	var result RotateResult
	rotated, err := logger.(AccessLogRotater).Rotate(&result)

	expect.NoError(t, err)
	expect.True(t, rotated)
	info, err := os.Stat(path)
	expect.NoError(t, err)
	expect.Equal(t, info.Mode().Perm(), os.FileMode(0o660))
}

func TestRotateKeepDaysFlushesSharedFileLoggers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")

	file1, err := OpenFile(path)
	expect.NoError(t, err)
	file2, err := OpenFile(path)
	expect.NoError(t, err)

	cfg := &RequestLoggerConfig{
		Format: FormatJSON,
		ConfigBase: ConfigBase{
			Retention: strutils.MustParse[*Retention]("30 days"),
		},
	}
	logger1 := NewFileAccessLogger(task.RootTask("test", false), file1, cfg)
	defer logger1.Close()
	logger2 := NewFileAccessLogger(task.RootTask("test", false), file2, cfg)
	defer logger2.Close()

	mockable.MockTimeNow(testTime)
	logger1.LogRequest(req, resp)
	logger2.LogRequest(req, resp)

	var result RotateResult
	rotated, err := logger1.(AccessLogRotater).Rotate(&result)

	expect.NoError(t, err)
	expect.True(t, rotated)
	archived := expect.Must(os.ReadFile(result.Filename))
	expect.Equal(t, bytes.Count(archived, []byte("\n")), 2)
	active := expect.Must(os.ReadFile(path))
	expect.Equal(t, len(active), 0)
}

func TestRotateKeepDaysDeletesExpiredArchives(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	file, err := OpenFile(path)
	expect.NoError(t, err)

	logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
		Format: FormatJSON,
		ConfigBase: ConfigBase{
			Retention: strutils.MustParse[*Retention]("30 days"),
		},
	})
	defer logger.Close()

	cutoff := testTime.AddDate(0, 0, -29)
	oldArchive := path + "." + cutoff.Add(-time.Hour).UTC().Format("20060102T150405.000000000Z")
	freshArchive := path + "." + cutoff.Add(time.Hour).UTC().Format("20060102T150405.000000000Z")
	expect.NoError(t, os.WriteFile(oldArchive, []byte("old\n"), 0o644))
	expect.NoError(t, os.WriteFile(freshArchive, []byte("fresh\n"), 0o644))

	mockable.MockTimeNow(testTime)
	logger.LogRequest(req, resp)
	logger.Flush()

	var result RotateResult
	rotated, err := logger.(AccessLogRotater).Rotate(&result)

	expect.NoError(t, err)
	expect.True(t, rotated)
	_, err = os.Stat(oldArchive)
	expect.True(t, errors.Is(err, os.ErrNotExist), "expected old archive to be deleted")
	_, err = os.Stat(freshArchive)
	expect.NoError(t, err)
}

func TestRotateKeepFileSize(t *testing.T) {
	for _, format := range ReqLoggerFormats {
		t.Run(string(format)+" keep size no rotation", func(t *testing.T) {
			file := newMockFile(true)
			logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
				Format: format,
			})
			expect.Nil(t, logger.Config().Retention)
			nLines := 10
			for i := range nLines {
				mockable.MockTimeNow(testTime.AddDate(0, 0, -nLines+i+1))
				logger.LogRequest(req, resp)
			}
			logger.Flush()
			expect.Equal(t, file.NumLines(), nLines)

			retention := strutils.MustParse[*Retention]("100 KB")
			expect.Equal(t, retention.KeepSize, 100*1024)
			expect.Equal(t, retention.Days, 0)
			expect.Equal(t, retention.Last, 0)
			logger.Config().Retention = retention

			mockable.MockTimeNow(testTime)
			var result RotateResult
			rotated, err := logger.(AccessLogRotater).Rotate(&result)
			expect.NoError(t, err)

			// file should be untouched as 100KB > 10 lines * bytes per line
			expect.Equal(t, rotated, false)
			expect.Equal(t, result.NumBytesKeep, file.Len())
			expect.Equal(t, result.NumBytesRead, 0, "should not read any bytes")
		})
	}

	t.Run("keep size with rotation", func(t *testing.T) {
		file := newMockFile(true)
		logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
			Format: FormatJSON,
		})
		expect.Nil(t, logger.Config().Retention)
		nLines := 100
		for i := range nLines {
			mockable.MockTimeNow(testTime.AddDate(0, 0, -nLines+i+1))
			logger.LogRequest(req, resp)
		}
		logger.Flush()
		expect.Equal(t, file.NumLines(), nLines)

		retention := strutils.MustParse[*Retention]("10 KB")
		expect.Equal(t, retention.KeepSize, 10*1024)
		expect.Equal(t, retention.Days, 0)
		expect.Equal(t, retention.Last, 0)
		logger.Config().Retention = retention

		mockable.MockTimeNow(testTime)
		var result RotateResult
		rotated, err := logger.(AccessLogRotater).Rotate(&result)
		expect.NoError(t, err)
		expect.Equal(t, rotated, true)
		expect.Equal(t, result.NumBytesKeep, int64(retention.KeepSize))
		expect.Equal(t, file.Len(), int64(retention.KeepSize))
		expect.Equal(t, result.NumBytesRead, 0, "should not read any bytes")
	})
}

// skipping invalid lines is not supported for keep file_size
func TestRotateSkipInvalidTime(t *testing.T) {
	for _, format := range ReqLoggerFormats {
		t.Run(string(format), func(t *testing.T) {
			file := newMockFile(true)
			logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
				Format: format,
			})
			expect.Nil(t, logger.Config().Retention)
			nLines := 10
			for i := range nLines {
				mockable.MockTimeNow(testTime.AddDate(0, 0, -nLines+i+1))
				logger.LogRequest(req, resp)
				logger.Flush()

				n, err := file.Write([]byte("invalid time\n"))
				expect.NoError(t, err)
				expect.Equal(t, n, len("invalid time\n"))
			}
			expect.Equal(t, file.NumLines(), 2*nLines)

			retention := strutils.MustParse[*Retention]("3 days")
			expect.Equal(t, retention.Days, 3)
			expect.Equal(t, retention.Last, 0)
			logger.Config().Retention = retention

			var result RotateResult
			rotated, err := logger.(AccessLogRotater).Rotate(&result)
			expect.NoError(t, err)
			expect.Equal(t, rotated, true)
			// should read one invalid line after every valid line
			expect.Equal(t, result.NumLinesKeep, int(retention.Days))
			expect.Equal(t, result.NumLinesInvalid, nLines-int(retention.Days)*2)
			expect.Equal(t, file.NumLines(), int(retention.Days))
		})
	}
}

func BenchmarkRotate(b *testing.B) {
	tests := []*Retention{
		{Days: 30},
		{Last: 100},
		{KeepSize: 24 * 1024},
	}
	for _, retention := range tests {
		b.Run(fmt.Sprintf("retention_%s", retention.String()), func(b *testing.B) {
			file := newMockFile(true)
			logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
				ConfigBase: ConfigBase{
					Retention: retention,
				},
				Format: FormatJSON,
			})
			for i := range 100 {
				mockable.MockTimeNow(testTime.AddDate(0, 0, -100+i+1))
				logger.LogRequest(req, resp)
			}
			logger.Flush()
			content := file.Content()
			b.ResetTimer()
			for b.Loop() {
				b.StopTimer()
				file = newMockFile(true)
				_, _ = file.Write(content)
				b.StartTimer()
				var result RotateResult
				_, _ = logger.(AccessLogRotater).Rotate(&result)
			}
		})
	}
}

func BenchmarkRotateWithInvalidTime(b *testing.B) {
	tests := []*Retention{
		{Days: 30},
		{Last: 100},
		{KeepSize: 24 * 1024},
	}
	for _, retention := range tests {
		b.Run(fmt.Sprintf("retention_%s", retention.String()), func(b *testing.B) {
			file := newMockFile(true)
			logger := NewFileAccessLogger(task.RootTask("test", false), file, &RequestLoggerConfig{
				ConfigBase: ConfigBase{
					Retention: retention,
				},
				Format: FormatJSON,
			})
			for i := range 10000 {
				mockable.MockTimeNow(testTime.AddDate(0, 0, -10000+i+1))
				logger.LogRequest(req, resp)
				if i%10 == 0 {
					_, _ = file.Write([]byte("invalid time\n"))
				}
			}
			logger.Flush()
			content := file.Content()
			b.ResetTimer()
			for b.Loop() {
				b.StopTimer()
				file = newMockFile(true)
				_, _ = file.Write(content)
				b.StartTimer()
				var result RotateResult
				_, _ = logger.(AccessLogRotater).Rotate(&result)
			}
		})
	}
}

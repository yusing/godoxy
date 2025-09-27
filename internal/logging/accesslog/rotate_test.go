package accesslog_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	. "github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/utils"
	expect "github.com/yusing/goutils/testing"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
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
			file := NewMockFile()
			utils.MockTimeNow(testTime)
			logger := NewAccessLoggerWithIO(task.RootTask("test", false), file, &RequestLoggerConfig{
				Format: format,
			})
			expect.Nil(t, logger.Config().Retention)

			for range 10 {
				logger.Log(req, resp)
			}
			logger.Flush()

			expect.Greater(t, file.Len(), int64(0))
			expect.Equal(t, file.NumLines(), 10)

			retention := strutils.MustParse[*Retention]("last 5")
			expect.Equal(t, retention.Days, 0)
			expect.Equal(t, retention.Last, 5)
			expect.Equal(t, retention.KeepSize, 0)
			logger.Config().Retention = retention

			result, err := logger.Rotate()
			expect.NoError(t, err)
			expect.Equal(t, file.NumLines(), int(retention.Last))
			expect.Equal(t, result.NumLinesKeep, int(retention.Last))
			expect.Equal(t, result.NumLinesInvalid, 0)
		})

		t.Run(string(format)+" keep days", func(t *testing.T) {
			file := NewMockFile()
			logger := NewAccessLoggerWithIO(task.RootTask("test", false), file, &RequestLoggerConfig{
				Format: format,
			})
			expect.Nil(t, logger.Config().Retention)
			nLines := 10
			for i := range nLines {
				utils.MockTimeNow(testTime.AddDate(0, 0, -nLines+i+1))
				logger.Log(req, resp)
			}
			logger.Flush()
			expect.Equal(t, file.NumLines(), nLines)

			retention := strutils.MustParse[*Retention]("3 days")
			expect.Equal(t, retention.Days, 3)
			expect.Equal(t, retention.Last, 0)
			expect.Equal(t, retention.KeepSize, 0)
			logger.Config().Retention = retention

			utils.MockTimeNow(testTime)
			result, err := logger.Rotate()
			expect.NoError(t, err)
			expect.Equal(t, file.NumLines(), int(retention.Days))
			expect.Equal(t, result.NumLinesKeep, int(retention.Days))
			expect.Equal(t, result.NumLinesInvalid, 0)

			rotated := file.Content()
			rotatedLines := bytes.Split(rotated, []byte("\n"))
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

func TestRotateKeepFileSize(t *testing.T) {
	for _, format := range ReqLoggerFormats {
		t.Run(string(format)+" keep size no rotation", func(t *testing.T) {
			file := NewMockFile()
			logger := NewAccessLoggerWithIO(task.RootTask("test", false), file, &RequestLoggerConfig{
				Format: format,
			})
			expect.Nil(t, logger.Config().Retention)
			nLines := 10
			for i := range nLines {
				utils.MockTimeNow(testTime.AddDate(0, 0, -nLines+i+1))
				logger.Log(req, resp)
			}
			logger.Flush()
			expect.Equal(t, file.NumLines(), nLines)

			retention := strutils.MustParse[*Retention]("100 KB")
			expect.Equal(t, retention.KeepSize, 100*1024)
			expect.Equal(t, retention.Days, 0)
			expect.Equal(t, retention.Last, 0)
			logger.Config().Retention = retention

			utils.MockTimeNow(testTime)
			result, err := logger.Rotate()
			expect.NoError(t, err)

			// file should be untouched as 100KB > 10 lines * bytes per line
			expect.Equal(t, result.NumBytesKeep, file.Len())
			expect.Equal(t, result.NumBytesRead, 0, "should not read any bytes")
		})
	}

	t.Run("keep size with rotation", func(t *testing.T) {
		file := NewMockFile()
		logger := NewAccessLoggerWithIO(task.RootTask("test", false), file, &RequestLoggerConfig{
			Format: FormatJSON,
		})
		expect.Nil(t, logger.Config().Retention)
		nLines := 100
		for i := range nLines {
			utils.MockTimeNow(testTime.AddDate(0, 0, -nLines+i+1))
			logger.Log(req, resp)
		}
		logger.Flush()
		expect.Equal(t, file.NumLines(), nLines)

		retention := strutils.MustParse[*Retention]("10 KB")
		expect.Equal(t, retention.KeepSize, 10*1024)
		expect.Equal(t, retention.Days, 0)
		expect.Equal(t, retention.Last, 0)
		logger.Config().Retention = retention

		utils.MockTimeNow(testTime)
		result, err := logger.Rotate()
		expect.NoError(t, err)
		expect.Equal(t, result.NumBytesKeep, int64(retention.KeepSize))
		expect.Equal(t, file.Len(), int64(retention.KeepSize))
		expect.Equal(t, result.NumBytesRead, 0, "should not read any bytes")
	})
}

// skipping invalid lines is not supported for keep file_size
func TestRotateSkipInvalidTime(t *testing.T) {
	for _, format := range ReqLoggerFormats {
		t.Run(string(format), func(t *testing.T) {
			file := NewMockFile()
			logger := NewAccessLoggerWithIO(task.RootTask("test", false), file, &RequestLoggerConfig{
				Format: format,
			})
			expect.Nil(t, logger.Config().Retention)
			nLines := 10
			for i := range nLines {
				utils.MockTimeNow(testTime.AddDate(0, 0, -nLines+i+1))
				logger.Log(req, resp)
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

			result, err := logger.Rotate()
			expect.NoError(t, err)
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
		b.Run(fmt.Sprintf("retention_%s", retention), func(b *testing.B) {
			file := NewMockFile()
			logger := NewAccessLoggerWithIO(task.RootTask("test", false), file, &RequestLoggerConfig{
				ConfigBase: ConfigBase{
					Retention: retention,
				},
				Format: FormatJSON,
			})
			for i := range 100 {
				utils.MockTimeNow(testTime.AddDate(0, 0, -100+i+1))
				logger.Log(req, resp)
			}
			logger.Flush()
			content := file.Content()
			b.ResetTimer()
			for b.Loop() {
				b.StopTimer()
				file = NewMockFile()
				_, _ = file.Write(content)
				b.StartTimer()
				_, _ = logger.Rotate()
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
		b.Run(fmt.Sprintf("retention_%s", retention), func(b *testing.B) {
			file := NewMockFile()
			logger := NewAccessLoggerWithIO(task.RootTask("test", false), file, &RequestLoggerConfig{
				ConfigBase: ConfigBase{
					Retention: retention,
				},
				Format: FormatJSON,
			})
			for i := range 10000 {
				utils.MockTimeNow(testTime.AddDate(0, 0, -10000+i+1))
				logger.Log(req, resp)
				if i%10 == 0 {
					_, _ = file.Write([]byte("invalid time\n"))
				}
			}
			logger.Flush()
			content := file.Content()
			b.ResetTimer()
			for b.Loop() {
				b.StopTimer()
				file = NewMockFile()
				_, _ = file.Write(content)
				b.StartTimer()
				_, _ = logger.Rotate()
			}
		})
	}
}

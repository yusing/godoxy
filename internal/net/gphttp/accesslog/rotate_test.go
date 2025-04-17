package accesslog_test

import (
	"fmt"
	"testing"
	"time"

	. "github.com/yusing/go-proxy/internal/net/gphttp/accesslog"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestParseLogTime(t *testing.T) {
	tests := []string{
		`{"foo":"bar","time":"%s","bar":"baz"}`,
		`example.com 192.168.1.1 - - [%s] "GET / HTTP/1.1" 200 1234`,
	}
	testTime := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	testTimeStr := testTime.Format(LogTimeFormat)

	for i, test := range tests {
		tests[i] = fmt.Sprintf(test, testTimeStr)
	}

	for _, test := range tests {
		t.Run(test, func(t *testing.T) {
			actual := ParseLogTime([]byte(test))
			expect.True(t, actual.Equal(testTime))
		})
	}
}

func TestRetentionCommonFormat(t *testing.T) {
	var file MockFile
	logger := NewAccessLoggerWithIO(task.RootTask("test", false), &file, &Config{
		Format:     FormatCommon,
		BufferSize: 1024,
	})
	for range 10 {
		logger.Log(req, resp)
	}
	logger.Flush()
	// test.Finish(nil)

	expect.Equal(t, logger.Config().Retention, nil)
	expect.True(t, file.Len() > 0)
	expect.Equal(t, file.LineCount(), 10)

	t.Run("keep last", func(t *testing.T) {
		logger.Config().Retention = strutils.MustParse[*Retention]("last 5")
		expect.Equal(t, logger.Config().Retention.Days, 0)
		expect.Equal(t, logger.Config().Retention.Last, 5)
		expect.NoError(t, logger.Rotate())
		expect.Equal(t, file.LineCount(), 5)
	})

	_ = file.Truncate(0)

	timeNow := time.Now()
	for i := range 10 {
		logger.Formatter.(*CommonFormatter).GetTimeNow = func() time.Time {
			return timeNow.AddDate(0, 0, -10+i)
		}
		logger.Log(req, resp)
	}
	logger.Flush()
	expect.Equal(t, file.LineCount(), 10)

	t.Run("keep days", func(t *testing.T) {
		logger.Config().Retention = strutils.MustParse[*Retention]("3 days")
		expect.Equal(t, logger.Config().Retention.Days, 3)
		expect.Equal(t, logger.Config().Retention.Last, 0)
		expect.NoError(t, logger.Rotate())
		expect.Equal(t, file.LineCount(), 3)
		rotated := string(file.Content())
		_ = file.Truncate(0)
		for i := range 3 {
			logger.Formatter.(*CommonFormatter).GetTimeNow = func() time.Time {
				return timeNow.AddDate(0, 0, -3+i)
			}
			logger.Log(req, resp)
		}
		expect.Equal(t, rotated, string(file.Content()))
	})
}

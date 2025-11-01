package accesslog

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"testing"

	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/goutils/task"
	expect "github.com/yusing/goutils/testing"
)

func TestNewMultiAccessLogger(t *testing.T) {
	testTask := task.RootTask("test", false)
	cfg := DefaultRequestLoggerConfig()

	writers := []Writer{
		NewMockFile(true),
		NewMockFile(true),
	}

	logger := NewMultiAccessLogger(testTask, cfg, writers)
	expect.NotNil(t, logger)
}

func TestMultiAccessLoggerConfig(t *testing.T) {
	testTask := task.RootTask("test", false)
	cfg := DefaultRequestLoggerConfig()
	cfg.Format = FormatCommon

	writers := []Writer{
		NewMockFile(true),
		NewMockFile(true),
	}

	logger := NewMultiAccessLogger(testTask, cfg, writers)
	retrievedCfg := logger.Config()

	expect.Equal(t, retrievedCfg.req.Format, FormatCommon)
}

func TestMultiAccessLoggerLog(t *testing.T) {
	testTask := task.RootTask("test", false)
	cfg := DefaultRequestLoggerConfig()
	cfg.Format = FormatCommon

	writer1 := NewMockFile(true)
	writer2 := NewMockFile(true)
	writers := []Writer{writer1, writer2}

	logger := NewMultiAccessLogger(testTask, cfg, writers)

	testURL, _ := url.Parse("http://example.com/test")
	req := &http.Request{
		RemoteAddr: "192.168.1.1",
		Method:     http.MethodGet,
		Proto:      "HTTP/1.1",
		Host:       "example.com",
		URL:        testURL,
		Header: http.Header{
			"User-Agent": []string{"test-agent"},
		},
	}
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		ContentLength: 100,
	}

	logger.Log(req, resp)
	logger.Flush()

	expect.Equal(t, writer1.NumLines(), 1)
	expect.Equal(t, writer2.NumLines(), 1)
}

func TestMultiAccessLoggerLogError(t *testing.T) {
	testTask := task.RootTask("test", false)
	cfg := DefaultRequestLoggerConfig()

	writer1 := NewMockFile(true)
	writer2 := NewMockFile(true)
	writers := []Writer{writer1, writer2}

	logger := NewMultiAccessLogger(testTask, cfg, writers)

	testURL, _ := url.Parse("http://example.com/test")
	req := &http.Request{
		RemoteAddr: "192.168.1.1",
		Method:     http.MethodGet,
		URL:        testURL,
	}
	testErr := errors.New("test error")

	logger.LogError(req, testErr)
	logger.Flush()

	expect.Equal(t, writer1.NumLines(), 1)
	expect.Equal(t, writer2.NumLines(), 1)
}

func TestMultiAccessLoggerLogACL(t *testing.T) {
	testTask := task.RootTask("test", false)
	cfg := DefaultACLLoggerConfig()
	cfg.LogAllowed = true

	writer1 := NewMockFile(true)
	writer2 := NewMockFile(true)
	writers := []Writer{writer1, writer2}

	logger := NewMultiAccessLogger(testTask, cfg, writers)

	info := &maxmind.IPInfo{
		IP:  net.ParseIP("192.168.1.1"),
		Str: "192.168.1.1",
	}

	logger.LogACL(info, false)
	logger.Flush()

	expect.Equal(t, writer1.NumLines(), 1)
	expect.Equal(t, writer2.NumLines(), 1)
}

func TestMultiAccessLoggerFlush(t *testing.T) {
	testTask := task.RootTask("test", false)
	cfg := DefaultRequestLoggerConfig()

	writer1 := NewMockFile(true)
	writer2 := NewMockFile(true)
	writers := []Writer{writer1, writer2}

	logger := NewMultiAccessLogger(testTask, cfg, writers)

	testURL, _ := url.Parse("http://example.com/test")
	req := &http.Request{
		RemoteAddr: "192.168.1.1",
		Method:     http.MethodGet,
		URL:        testURL,
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
	}

	logger.Log(req, resp)
	logger.Flush()

	expect.Equal(t, writer1.NumLines(), 1)
	expect.Equal(t, writer2.NumLines(), 1)
}

func TestMultiAccessLoggerClose(t *testing.T) {
	testTask := task.RootTask("test", false)
	cfg := DefaultRequestLoggerConfig()

	writer1 := NewMockFile(true)
	writer2 := NewMockFile(true)
	writers := []Writer{writer1, writer2}

	logger := NewMultiAccessLogger(testTask, cfg, writers)

	err := logger.Close()
	expect.Nil(t, err)
}

func TestMultiAccessLoggerMultipleLogs(t *testing.T) {
	testTask := task.RootTask("test", false)
	cfg := DefaultRequestLoggerConfig()

	writer1 := NewMockFile(true)
	writer2 := NewMockFile(true)
	writers := []Writer{writer1, writer2}

	logger := NewMultiAccessLogger(testTask, cfg, writers)

	testURL, _ := url.Parse("http://example.com/test")

	for range 3 {
		req := &http.Request{
			RemoteAddr: "192.168.1.1",
			Method:     http.MethodGet,
			URL:        testURL,
		}
		resp := &http.Response{
			StatusCode: http.StatusOK,
		}
		logger.Log(req, resp)
	}

	logger.Flush()

	expect.Equal(t, writer1.NumLines(), 3)
	expect.Equal(t, writer2.NumLines(), 3)
}

func TestMultiAccessLoggerSingleWriter(t *testing.T) {
	testTask := task.RootTask("test", false)
	cfg := DefaultRequestLoggerConfig()

	writer := NewMockFile(true)
	writers := []Writer{writer}

	logger := NewMultiAccessLogger(testTask, cfg, writers)
	expect.NotNil(t, logger)

	testURL, _ := url.Parse("http://example.com/test")
	req := &http.Request{
		RemoteAddr: "192.168.1.1",
		Method:     http.MethodGet,
		URL:        testURL,
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
	}

	logger.Log(req, resp)
	logger.Flush()

	expect.Equal(t, writer.NumLines(), 1)
}

func TestMultiAccessLoggerMixedOperations(t *testing.T) {
	testTask := task.RootTask("test", false)
	cfg := DefaultRequestLoggerConfig()

	writer1 := NewMockFile(true)
	writer2 := NewMockFile(true)
	writers := []Writer{writer1, writer2}

	logger := NewMultiAccessLogger(testTask, cfg, writers)

	testURL, _ := url.Parse("http://example.com/test")

	req := &http.Request{
		RemoteAddr: "192.168.1.1",
		Method:     http.MethodGet,
		URL:        testURL,
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
	}

	logger.Log(req, resp)
	logger.Flush()

	info := &maxmind.IPInfo{
		IP:  net.ParseIP("192.168.1.1"),
		Str: "192.168.1.1",
	}

	cfg2 := DefaultACLLoggerConfig()
	cfg2.LogAllowed = true
	aclLogger := NewMultiAccessLogger(testTask, cfg2, writers)
	aclLogger.LogACL(info, false)

	logger.Flush()

	expect.Equal(t, writer1.NumLines(), 1)
	expect.Equal(t, writer2.NumLines(), 1)
}

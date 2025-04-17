package accesslog_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/yusing/go-proxy/pkg/json"

	. "github.com/yusing/go-proxy/internal/net/gphttp/accesslog"
	"github.com/yusing/go-proxy/internal/task"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

const (
	remote        = "192.168.1.1"
	host          = "example.com"
	uri           = "/?bar=baz&foo=bar"
	uriRedacted   = "/?bar=" + RedactedValue + "&foo=" + RedactedValue
	referer       = "https://www.google.com/"
	proto         = "HTTP/1.1"
	ua            = "Go-http-client/1.1"
	status        = http.StatusNotFound
	contentLength = 100
	method        = http.MethodGet
)

var (
	testTask = task.RootTask("test", false)
	testURL  = expect.Must(url.Parse("http://" + host + uri))
	req      = &http.Request{
		RemoteAddr: remote,
		Method:     method,
		Proto:      proto,
		Host:       testURL.Host,
		URL:        testURL,
		Header: http.Header{
			"User-Agent": []string{ua},
			"Referer":    []string{referer},
			"Cookie": []string{
				"foo=bar",
				"bar=baz",
			},
		},
	}
	resp = &http.Response{
		StatusCode:    status,
		ContentLength: contentLength,
		Header:        http.Header{"Content-Type": []string{"text/plain"}},
	}
)

func fmtLog(cfg *Config) (ts string, line string) {
	var buf bytes.Buffer

	t := time.Now()
	logger := NewMockAccessLogger(testTask, cfg)
	logger.Formatter.SetGetTimeNow(func() time.Time {
		return t
	})
	logger.Format(&buf, req, resp)
	return t.Format(LogTimeFormat), buf.String()
}

func TestAccessLoggerCommon(t *testing.T) {
	config := DefaultConfig()
	config.Format = FormatCommon
	ts, log := fmtLog(config)
	expect.Equal(t, log,
		fmt.Sprintf("%s %s - - [%s] \"%s %s %s\" %d %d",
			host, remote, ts, method, uri, proto, status, contentLength,
		),
	)
}

func TestAccessLoggerCombined(t *testing.T) {
	config := DefaultConfig()
	config.Format = FormatCombined
	ts, log := fmtLog(config)
	expect.Equal(t, log,
		fmt.Sprintf("%s %s - - [%s] \"%s %s %s\" %d %d \"%s\" \"%s\"",
			host, remote, ts, method, uri, proto, status, contentLength, referer, ua,
		),
	)
}

func TestAccessLoggerRedactQuery(t *testing.T) {
	config := DefaultConfig()
	config.Format = FormatCommon
	config.Fields.Query.Default = FieldModeRedact
	ts, log := fmtLog(config)
	expect.Equal(t, log,
		fmt.Sprintf("%s %s - - [%s] \"%s %s %s\" %d %d",
			host, remote, ts, method, uriRedacted, proto, status, contentLength,
		),
	)
}

type JSONLogEntry struct {
	Time        string              `json:"time"`
	IP          string              `json:"ip"`
	Method      string              `json:"method"`
	Scheme      string              `json:"scheme"`
	Host        string              `json:"host"`
	URI         string              `json:"uri"`
	Protocol    string              `json:"protocol"`
	Status      int                 `json:"status"`
	Error       string              `json:"error,omitempty"`
	ContentType string              `json:"type"`
	Size        int64               `json:"size"`
	Referer     string              `json:"referer"`
	UserAgent   string              `json:"useragent"`
	Query       map[string][]string `json:"query,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
	Cookies     map[string]string   `json:"cookies,omitempty"`
}

func getJSONEntry(t *testing.T, config *Config) JSONLogEntry {
	t.Helper()
	config.Format = FormatJSON
	var entry JSONLogEntry
	_, log := fmtLog(config)
	err := json.Unmarshal([]byte(log), &entry)
	expect.NoError(t, err)
	return entry
}

func TestAccessLoggerJSON(t *testing.T) {
	config := DefaultConfig()
	entry := getJSONEntry(t, config)
	expect.Equal(t, entry.IP, remote)
	expect.Equal(t, entry.Method, method)
	expect.Equal(t, entry.Scheme, "http")
	expect.Equal(t, entry.Host, testURL.Host)
	expect.Equal(t, entry.URI, testURL.RequestURI())
	expect.Equal(t, entry.Protocol, proto)
	expect.Equal(t, entry.Status, status)
	expect.Equal(t, entry.ContentType, "text/plain")
	expect.Equal(t, entry.Size, contentLength)
	expect.Equal(t, entry.Referer, referer)
	expect.Equal(t, entry.UserAgent, ua)
	expect.Equal(t, len(entry.Headers), 0)
	expect.Equal(t, len(entry.Cookies), 0)
	if status >= 400 {
		expect.Equal(t, entry.Error, http.StatusText(status))
	}
}

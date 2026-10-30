package accesslog_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	. "github.com/yusing/godoxy/internal/logging/accesslog"
	expect "github.com/yusing/goutils/testing"
)

func formatRequest(t *testing.T, format Format, r *http.Request) string {
	t.Helper()
	cfg := DefaultRequestLoggerConfig()
	cfg.Format = format
	logger := newMockAccessLogger(testTask, cfg)
	var buf bytes.Buffer
	logger.(RequestFormatter).AppendRequestLog(&buf, r, resp)
	return buf.String()
}

func jsonFields(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	expect.NoError(t, json.Unmarshal([]byte(line), &m))
	return m
}

func TestRequestInfoDenialAndPeerIP(t *testing.T) {
	r := req.Clone(t.Context())
	r.RemoteAddr = "127.0.0.1:40000"
	r = WithRequestInfo(r)
	expect.Equal(t, WithRequestInfo(r), r)

	// a real IP middleware rewrites the client address
	r.RemoteAddr = "203.0.113.7:40000"
	RecordDenied(r, "CIDRWhitelist", "IP not allowed")
	RecordDenied(r, "RateLimit", "rate limit exceeded")

	info, ok := RequestInfoOf(r)
	expect.True(t, ok)
	expect.Equal(t, info, RequestInfo{PeerIP: "127.0.0.1", DeniedBy: "CIDRWhitelist", DenyReason: "IP not allowed"})

	m := jsonFields(t, formatRequest(t, FormatJSON, r))
	expect.Equal(t, m["ip"], "203.0.113.7")
	expect.Equal(t, m["peer_ip"], "127.0.0.1")
	expect.Equal(t, m["denied_by"], "CIDRWhitelist")
	expect.Equal(t, m["deny_reason"], "IP not allowed")

	for _, format := range []Format{FormatCommon, FormatCombined} {
		line := formatRequest(t, format, r)
		expect.False(t, strings.Contains(line, "CIDRWhitelist"), "text formats stay unchanged")
		expect.False(t, strings.Contains(line, "127.0.0.1"), "text formats stay unchanged")
	}
}

func TestRequestInfoOmittedWhenNothingToAdd(t *testing.T) {
	r := WithRequestInfo(req.Clone(t.Context()))
	m := jsonFields(t, formatRequest(t, FormatJSON, r))
	for _, key := range []string{"peer_ip", "denied_by", "deny_reason"} {
		_, ok := m[key]
		expect.False(t, ok, key)
	}
}

func TestRecordDeniedWithoutSlotIsNoop(t *testing.T) {
	r := req.Clone(t.Context())
	RecordDenied(r, "CIDRWhitelist", "IP not allowed")
	_, ok := RequestInfoOf(r)
	expect.False(t, ok)
	_, has := jsonFields(t, formatRequest(t, FormatJSON, r))["denied_by"]
	expect.False(t, has)
}

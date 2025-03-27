package accesslog_test

import (
	"testing"

	. "github.com/yusing/go-proxy/internal/net/gphttp/accesslog"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

// Cookie header should be removed,
// stored in JSONLogEntry.Cookies instead.
func TestAccessLoggerJSONKeepHeaders(t *testing.T) {
	config := DefaultConfig()
	config.Fields.Headers.Default = FieldModeKeep
	entry := getJSONEntry(t, config)
	for k, v := range req.Header {
		if k != "Cookie" {
			ExpectDeepEqual(t, entry.Headers[k], v)
		}
	}

	config.Fields.Headers.Config = map[string]FieldMode{
		"Referer":    FieldModeRedact,
		"User-Agent": FieldModeDrop,
	}
	entry = getJSONEntry(t, config)
	ExpectDeepEqual(t, entry.Headers["Referer"], []string{RedactedValue})
	ExpectDeepEqual(t, entry.Headers["User-Agent"], nil)
}

func TestAccessLoggerJSONDropHeaders(t *testing.T) {
	config := DefaultConfig()
	config.Fields.Headers.Default = FieldModeDrop
	entry := getJSONEntry(t, config)
	for k := range req.Header {
		ExpectDeepEqual(t, entry.Headers[k], nil)
	}

	config.Fields.Headers.Config = map[string]FieldMode{
		"Referer":    FieldModeKeep,
		"User-Agent": FieldModeRedact,
	}
	entry = getJSONEntry(t, config)
	ExpectDeepEqual(t, entry.Headers["Referer"], []string{req.Header.Get("Referer")})
	ExpectDeepEqual(t, entry.Headers["User-Agent"], []string{RedactedValue})
}

func TestAccessLoggerJSONRedactHeaders(t *testing.T) {
	config := DefaultConfig()
	config.Fields.Headers.Default = FieldModeRedact
	entry := getJSONEntry(t, config)
	ExpectEqual(t, len(entry.Headers["Cookie"]), 0)
	for k := range req.Header {
		if k != "Cookie" {
			ExpectDeepEqual(t, entry.Headers[k], []string{RedactedValue})
		}
	}
}

func TestAccessLoggerJSONKeepCookies(t *testing.T) {
	config := DefaultConfig()
	config.Fields.Headers.Default = FieldModeKeep
	config.Fields.Cookies.Default = FieldModeKeep
	entry := getJSONEntry(t, config)
	ExpectEqual(t, len(entry.Headers["Cookie"]), 0)
	for _, cookie := range req.Cookies() {
		ExpectEqual(t, entry.Cookies[cookie.Name], cookie.Value)
	}
}

func TestAccessLoggerJSONRedactCookies(t *testing.T) {
	config := DefaultConfig()
	config.Fields.Headers.Default = FieldModeKeep
	config.Fields.Cookies.Default = FieldModeRedact
	entry := getJSONEntry(t, config)
	ExpectEqual(t, len(entry.Headers["Cookie"]), 0)
	for _, cookie := range req.Cookies() {
		ExpectEqual(t, entry.Cookies[cookie.Name], RedactedValue)
	}
}

func TestAccessLoggerJSONDropQuery(t *testing.T) {
	config := DefaultConfig()
	config.Fields.Query.Default = FieldModeDrop
	entry := getJSONEntry(t, config)
	ExpectDeepEqual(t, entry.Query["foo"], nil)
	ExpectDeepEqual(t, entry.Query["bar"], nil)
}

func TestAccessLoggerJSONRedactQuery(t *testing.T) {
	config := DefaultConfig()
	config.Fields.Query.Default = FieldModeRedact
	entry := getJSONEntry(t, config)
	ExpectDeepEqual(t, entry.Query["foo"], []string{RedactedValue})
	ExpectDeepEqual(t, entry.Query["bar"], []string{RedactedValue})
}

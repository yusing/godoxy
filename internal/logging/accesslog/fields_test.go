package accesslog_test

import (
	"testing"

	. "github.com/yusing/godoxy/internal/logging/accesslog"
	expect "github.com/yusing/godoxy/internal/utils/testing"
)

// Cookie header should be removed,
// stored in JSONLogEntry.Cookies instead.
func TestAccessLoggerJSONKeepHeaders(t *testing.T) {
	config := DefaultRequestLoggerConfig()
	config.Fields.Headers.Default = FieldModeKeep
	entry := getJSONEntry(t, config)
	for k, v := range req.Header {
		if k != "Cookie" {
			expect.Equal(t, entry.Headers[k], v)
		}
	}

	config.Fields.Headers.Config = map[string]FieldMode{
		"Referer":    FieldModeRedact,
		"User-Agent": FieldModeDrop,
	}
	entry = getJSONEntry(t, config)
	expect.Equal(t, entry.Headers["Referer"], []string{RedactedValue})
	expect.Equal(t, entry.Headers["User-Agent"], nil)
}

func TestAccessLoggerJSONDropHeaders(t *testing.T) {
	config := DefaultRequestLoggerConfig()
	config.Fields.Headers.Default = FieldModeDrop
	entry := getJSONEntry(t, config)
	for k := range req.Header {
		expect.Equal(t, entry.Headers[k], nil)
	}

	config.Fields.Headers.Config = map[string]FieldMode{
		"Referer":    FieldModeKeep,
		"User-Agent": FieldModeRedact,
	}
	entry = getJSONEntry(t, config)
	expect.Equal(t, entry.Headers["Referer"], []string{req.Header.Get("Referer")})
	expect.Equal(t, entry.Headers["User-Agent"], []string{RedactedValue})
}

func TestAccessLoggerJSONRedactHeaders(t *testing.T) {
	config := DefaultRequestLoggerConfig()
	config.Fields.Headers.Default = FieldModeRedact
	entry := getJSONEntry(t, config)
	for k := range req.Header {
		if k != "Cookie" {
			expect.Equal(t, entry.Headers[k], []string{RedactedValue})
		}
	}
}

func TestAccessLoggerJSONKeepCookies(t *testing.T) {
	config := DefaultRequestLoggerConfig()
	config.Fields.Headers.Default = FieldModeKeep
	config.Fields.Cookies.Default = FieldModeKeep
	entry := getJSONEntry(t, config)
	for _, cookie := range req.Cookies() {
		expect.Equal(t, entry.Cookies[cookie.Name], cookie.Value)
	}
}

func TestAccessLoggerJSONRedactCookies(t *testing.T) {
	config := DefaultRequestLoggerConfig()
	config.Fields.Headers.Default = FieldModeKeep
	config.Fields.Cookies.Default = FieldModeRedact
	entry := getJSONEntry(t, config)
	for _, cookie := range req.Cookies() {
		expect.Equal(t, entry.Cookies[cookie.Name], RedactedValue)
	}
}

func TestAccessLoggerJSONDropQuery(t *testing.T) {
	config := DefaultRequestLoggerConfig()
	config.Fields.Query.Default = FieldModeDrop
	entry := getJSONEntry(t, config)
	expect.Equal(t, entry.Query["foo"], nil)
	expect.Equal(t, entry.Query["bar"], nil)
}

func TestAccessLoggerJSONRedactQuery(t *testing.T) {
	config := DefaultRequestLoggerConfig()
	config.Fields.Query.Default = FieldModeRedact
	entry := getJSONEntry(t, config)
	expect.Equal(t, entry.Query["foo"], []string{RedactedValue})
	expect.Equal(t, entry.Query["bar"], []string{RedactedValue})
}

package accesslog_test

import (
	"testing"

	"github.com/yusing/go-proxy/internal/docker"
	. "github.com/yusing/go-proxy/internal/logging/accesslog"
	"github.com/yusing/go-proxy/internal/utils"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestNewConfig(t *testing.T) {
	labels := map[string]string{
		"proxy.format":                      "combined",
		"proxy.path":                        "/tmp/access.log",
		"proxy.filters.status_codes.values": "200-299",
		"proxy.filters.method.values":       "GET, POST",
		"proxy.filters.headers.values":      "foo=bar, baz",
		"proxy.filters.headers.negative":    "true",
		"proxy.filters.cidr.values":         "192.168.10.0/24",
		"proxy.fields.headers.default":      "keep",
		"proxy.fields.headers.config.foo":   "redact",
		"proxy.fields.query.default":        "drop",
		"proxy.fields.query.config.foo":     "keep",
		"proxy.fields.cookies.default":      "redact",
		"proxy.fields.cookies.config.foo":   "keep",
	}
	parsed, err := docker.ParseLabels(labels)
	expect.NoError(t, err)

	var config RequestLoggerConfig
	err = utils.MapUnmarshalValidate(parsed, &config)
	expect.NoError(t, err)

	expect.Equal(t, config.Format, FormatCombined)
	expect.Equal(t, config.Path, "/tmp/access.log")
	expect.Equal(t, config.Filters.StatusCodes.Values, []*StatusCodeRange{{Start: 200, End: 299}})
	expect.Equal(t, len(config.Filters.Method.Values), 2)
	expect.Equal(t, config.Filters.Method.Values, []HTTPMethod{"GET", "POST"})
	expect.Equal(t, len(config.Filters.Headers.Values), 2)
	expect.Equal(t, config.Filters.Headers.Values, []*HTTPHeader{{Key: "foo", Value: "bar"}, {Key: "baz", Value: ""}})
	expect.True(t, config.Filters.Headers.Negative)
	expect.Equal(t, len(config.Filters.CIDR.Values), 1)
	expect.Equal(t, config.Filters.CIDR.Values[0].String(), "192.168.10.0/24")
	expect.Equal(t, config.Fields.Headers.Default, FieldModeKeep)
	expect.Equal(t, config.Fields.Headers.Config["foo"], FieldModeRedact)
	expect.Equal(t, config.Fields.Query.Default, FieldModeDrop)
	expect.Equal(t, config.Fields.Query.Config["foo"], FieldModeKeep)
	expect.Equal(t, config.Fields.Cookies.Default, FieldModeRedact)
	expect.Equal(t, config.Fields.Cookies.Config["foo"], FieldModeKeep)
}

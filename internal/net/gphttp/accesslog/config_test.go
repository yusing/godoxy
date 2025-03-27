package accesslog_test

import (
	"testing"

	"github.com/yusing/go-proxy/internal/docker"
	. "github.com/yusing/go-proxy/internal/net/gphttp/accesslog"
	"github.com/yusing/go-proxy/internal/utils"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestNewConfig(t *testing.T) {
	labels := map[string]string{
		"proxy.buffer_size":                 "10",
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
	ExpectNoError(t, err)

	var config Config
	err = utils.Deserialize(parsed, &config)
	ExpectNoError(t, err)

	ExpectEqual(t, config.BufferSize, 10)
	ExpectEqual(t, config.Format, FormatCombined)
	ExpectEqual(t, config.Path, "/tmp/access.log")
	ExpectDeepEqual(t, config.Filters.StatusCodes.Values, []*StatusCodeRange{{Start: 200, End: 299}})
	ExpectEqual(t, len(config.Filters.Method.Values), 2)
	ExpectDeepEqual(t, config.Filters.Method.Values, []HTTPMethod{"GET", "POST"})
	ExpectEqual(t, len(config.Filters.Headers.Values), 2)
	ExpectDeepEqual(t, config.Filters.Headers.Values, []*HTTPHeader{{Key: "foo", Value: "bar"}, {Key: "baz", Value: ""}})
	ExpectTrue(t, config.Filters.Headers.Negative)
	ExpectEqual(t, len(config.Filters.CIDR.Values), 1)
	ExpectEqual(t, config.Filters.CIDR.Values[0].String(), "192.168.10.0/24")
	ExpectEqual(t, config.Fields.Headers.Default, FieldModeKeep)
	ExpectEqual(t, config.Fields.Headers.Config["foo"], FieldModeRedact)
	ExpectEqual(t, config.Fields.Query.Default, FieldModeDrop)
	ExpectEqual(t, config.Fields.Query.Config["foo"], FieldModeKeep)
	ExpectEqual(t, config.Fields.Cookies.Default, FieldModeRedact)
	ExpectEqual(t, config.Fields.Cookies.Config["foo"], FieldModeKeep)
}

package docker_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/go-proxy/internal/docker"
)

func TestExpandWildcard(t *testing.T) {
	labels := map[string]string{
		"proxy.a.host":                "localhost",
		"proxy.b.port":                "4444",
		"proxy.b.scheme":              "http",
		"proxy.*.port":                "5555",
		"proxy.*.healthcheck.disable": "true",
	}

	docker.ExpandWildcard(labels)

	require.Equal(t, map[string]string{
		"proxy.a.host":                "localhost",
		"proxy.a.port":                "5555",
		"proxy.a.healthcheck.disable": "true",
		"proxy.b.scheme":              "http",
		"proxy.b.port":                "5555",
		"proxy.b.healthcheck.disable": "true",
	}, labels)
}

func TestExpandWildcardWithFQDNAliases(t *testing.T) {
	labels := map[string]string{
		"proxy.c.host": "localhost",
		"proxy.*.port": "5555",
	}
	docker.ExpandWildcard(labels, "a.example.com", "b.example.com")
	require.Equal(t, map[string]string{
		"proxy.#1.port": "5555",
		"proxy.#2.port": "5555",
		"proxy.c.host":  "localhost",
		"proxy.c.port":  "5555",
	}, labels)
}

func TestExpandWildcardYAML(t *testing.T) {
	yaml := `
host: localhost
port: 5555
healthcheck:
	disable: true`
	labels := map[string]string{
		"proxy.*":                     yaml[1:],
		"proxy.a.port":                "4444",
		"proxy.a.healthcheck.disable": "false",
		"proxy.a.healthcheck.path":    "/health",
		"proxy.b.port":                "6666",
	}
	docker.ExpandWildcard(labels)
	require.Equal(t, map[string]string{
		"proxy.a.host":                "localhost", // set by wildcard
		"proxy.a.port":                "5555",      // overridden by wildcard
		"proxy.a.healthcheck.disable": "true",      // overridden by wildcard
		"proxy.a.healthcheck.path":    "/health",   // own label
		"proxy.b.host":                "localhost", // set by wildcard
		"proxy.b.port":                "5555",      // overridden by wildcard
		"proxy.b.healthcheck.disable": "true",      // overridden by wildcard
	}, labels)
}

func BenchmarkParseLabels(b *testing.B) {
	for b.Loop() {
		_, _ = docker.ParseLabels(map[string]string{
			"proxy.a.host":   "localhost",
			"proxy.b.port":   "4444",
			"proxy.*.scheme": "http",
			"proxy.*.middlewares.request.hide_headers": "X-Header1,X-Header2",
		})
	}
}

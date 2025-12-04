package docker_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
)

func TestExpandWildcard(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		labels := map[string]string{
			"proxy.a.host":                "localhost",
			"proxy.b.port":                "4444",
			"proxy.b.scheme":              "http",
			"proxy.*.port":                "5555",
			"proxy.*.healthcheck.disable": "true",
		}
		docker.ExpandWildcard(labels, "a", "b")
		require.Equal(t, map[string]string{
			"proxy.#1.host":                "localhost",
			"proxy.#1.port":                "5555",
			"proxy.#1.healthcheck.disable": "true",
			"proxy.#2.port":                "5555",
			"proxy.#2.scheme":              "http",
			"proxy.#2.healthcheck.disable": "true",
		}, labels)
	})

	t.Run("no wildcards", func(t *testing.T) {
		labels := map[string]string{
			"proxy.a.host": "localhost",
			"proxy.b.port": "4444",
		}
		docker.ExpandWildcard(labels, "a", "b")
		require.Equal(t, map[string]string{
			"proxy.a.host": "localhost",
			"proxy.b.port": "4444",
		}, labels)
	})

	t.Run("no aliases", func(t *testing.T) {
		labels := map[string]string{
			"proxy.*.port": "5555",
		}
		docker.ExpandWildcard(labels)
		require.Equal(t, map[string]string{}, labels)
	})

	t.Run("empty labels", func(t *testing.T) {
		labels := map[string]string{}
		docker.ExpandWildcard(labels, "a", "b")
		require.Equal(t, map[string]string{}, labels)
	})

	t.Run("only wildcards no explicit labels", func(t *testing.T) {
		labels := map[string]string{
			"proxy.*.port":   "5555",
			"proxy.*.scheme": "https",
		}
		docker.ExpandWildcard(labels, "a", "b")
		require.Equal(t, map[string]string{
			"proxy.#1.port":   "5555",
			"proxy.#1.scheme": "https",
			"proxy.#2.port":   "5555",
			"proxy.#2.scheme": "https",
		}, labels)
	})

	t.Run("non-proxy labels unchanged", func(t *testing.T) {
		labels := map[string]string{
			"other.label":    "value",
			"proxy.*.port":   "5555",
			"proxy.a.scheme": "http",
		}
		docker.ExpandWildcard(labels, "a")
		require.Equal(t, map[string]string{
			"other.label":     "value",
			"proxy.#1.port":   "5555",
			"proxy.#1.scheme": "http",
		}, labels)
	})

	t.Run("single alias multiple labels", func(t *testing.T) {
		labels := map[string]string{
			"proxy.a.host":   "localhost",
			"proxy.a.port":   "8080",
			"proxy.a.scheme": "https",
			"proxy.*.port":   "5555",
		}
		docker.ExpandWildcard(labels, "a")
		require.Equal(t, map[string]string{
			"proxy.#1.host":   "localhost",
			"proxy.#1.port":   "5555",
			"proxy.#1.scheme": "https",
		}, labels)
	})

	t.Run("wildcard partial override", func(t *testing.T) {
		labels := map[string]string{
			"proxy.a.host":             "localhost",
			"proxy.a.port":             "8080",
			"proxy.a.healthcheck.path": "/health",
			"proxy.*.port":             "5555",
		}
		docker.ExpandWildcard(labels, "a")
		require.Equal(t, map[string]string{
			"proxy.#1.host":             "localhost",
			"proxy.#1.port":             "5555",
			"proxy.#1.healthcheck.path": "/health",
		}, labels)
	})

	t.Run("nested suffix distinction", func(t *testing.T) {
		labels := map[string]string{
			"proxy.a.healthcheck.path":     "/health",
			"proxy.a.healthcheck.interval": "10s",
			"proxy.*.healthcheck.disable":  "true",
		}
		docker.ExpandWildcard(labels, "a")
		require.Equal(t, map[string]string{
			"proxy.#1.healthcheck.path":     "/health",
			"proxy.#1.healthcheck.interval": "10s",
			"proxy.#1.healthcheck.disable":  "true",
		}, labels)
	})

	t.Run("discovered alias from explicit label", func(t *testing.T) {
		labels := map[string]string{
			"proxy.c.host": "localhost",
			"proxy.*.port": "5555",
		}
		docker.ExpandWildcard(labels, "a", "b")
		require.Equal(t, map[string]string{
			"proxy.#1.port": "5555",
			"proxy.#2.port": "5555",
			"proxy.#3.host": "localhost",
			"proxy.#3.port": "5555",
		}, labels)
	})

	t.Run("ref alias not converted", func(t *testing.T) {
		labels := map[string]string{
			"proxy.#1.host":  "localhost",
			"proxy.#2.port":  "8080",
			"proxy.*.scheme": "https",
		}
		docker.ExpandWildcard(labels, "a", "b")
		require.Equal(t, map[string]string{
			"proxy.#1.host":   "localhost",
			"proxy.#1.scheme": "https",
			"proxy.#2.port":   "8080",
			"proxy.#2.scheme": "https",
		}, labels)
	})

	t.Run("mixed ref and named aliases", func(t *testing.T) {
		labels := map[string]string{
			"proxy.#1.host": "host1",
			"proxy.a.host":  "host2",
			"proxy.*.port":  "5555",
		}
		docker.ExpandWildcard(labels, "a", "b")
		require.Equal(t, map[string]string{
			"proxy.#1.host": "host2",
			"proxy.#1.port": "5555",
			"proxy.#2.port": "5555",
		}, labels)
	})
}

func TestExpandWildcardYAML(t *testing.T) {
	t.Run("basic yaml wildcard", func(t *testing.T) {
		yaml := `
host: localhost
port: 5555
healthcheck:
	disable: true`[1:]
		labels := map[string]string{
			"proxy.*":                     yaml,
			"proxy.a.port":                "4444",
			"proxy.a.healthcheck.disable": "false",
			"proxy.a.healthcheck.path":    "/health",
			"proxy.b.port":                "6666",
		}
		docker.ExpandWildcard(labels, "a", "b")
		require.Equal(t, map[string]string{
			"proxy.#1.host":                "localhost",
			"proxy.#1.port":                "5555",
			"proxy.#1.healthcheck.disable": "true",
			"proxy.#1.healthcheck.path":    "/health",
			"proxy.#2.host":                "localhost",
			"proxy.#2.port":                "5555",
			"proxy.#2.healthcheck.disable": "true",
		}, labels)
	})

	t.Run("yaml with nested maps", func(t *testing.T) {
		yaml := `
middlewares:
	request:
		hide_headers: X-Secret
		add_headers:
			X-Custom: value`[1:]
		labels := map[string]string{
			"proxy.*": yaml,
			"proxy.a.middlewares.request.set_headers": "X-Override: yes",
		}
		docker.ExpandWildcard(labels, "a")
		require.Equal(t, map[string]string{
			"proxy.#1.middlewares.request.hide_headers":         "X-Secret",
			"proxy.#1.middlewares.request.add_headers.X-Custom": "value",
			"proxy.#1.middlewares.request.set_headers":          "X-Override: yes",
		}, labels)
	})

	t.Run("yaml only no explicit labels", func(t *testing.T) {
		yaml := `
host: localhost
port: 8080`[1:]
		labels := map[string]string{
			"proxy.*": yaml,
		}
		docker.ExpandWildcard(labels, "a", "b")
		require.Equal(t, map[string]string{
			"proxy.#1.host": "localhost",
			"proxy.#1.port": "8080",
			"proxy.#2.host": "localhost",
			"proxy.#2.port": "8080",
		}, labels)
	})

	t.Run("invalid yaml ignored", func(t *testing.T) {
		labels := map[string]string{
			"proxy.*":      "invalid: yaml: content:\n\t\tbad",
			"proxy.a.port": "8080",
		}
		docker.ExpandWildcard(labels, "a")
		require.Equal(t, map[string]string{
			"proxy.a.port": "8080",
		}, labels)
	})
}

func BenchmarkParseLabels(b *testing.B) {
	m := map[string]string{
		"proxy.a.host":   "localhost",
		"proxy.b.port":   "4444",
		"proxy.*.scheme": "http",
		"proxy.*.middlewares.request.hide_headers": "X-Header1,X-Header2",
	}
	for b.Loop() {
		_, _ = docker.ParseLabels(m, "a", "b")
	}
}

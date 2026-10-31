package middleware

import (
	"strings"
	"testing"

	strutils "github.com/yusing/goutils/strings"
	expect "github.com/yusing/goutils/testing"
)

func TestClientIPOrderWarnings(t *testing.T) {
	whitelist := map[string]any{"use": "cidr_whitelist", "allow": []string{"127.0.0.1/32"}}
	cfRealIP := map[string]any{"use": "cloudflare_real_ip"}
	redirect := map[string]any{"use": "redirect_http"}

	t.Run("whitelist_before_cloudflare_real_ip", func(t *testing.T) {
		// The chain from the 2026-10 report: cloudflared on 127.0.0.1 passed
		// the whitelist for every tunnel visitor.
		w := ClientIPOrderWarnings("entrypoint", []map[string]any{redirect, whitelist, cfRealIP})
		expect.Equal(t, len(w), 1)
		expect.True(t, strings.Contains(w[0], "entrypoint[1] cidr_whitelist reads the client IP"))
		expect.True(t, strings.Contains(w[0], "entrypoint[2] cloudflare_real_ip"))
	})

	t.Run("correct_order", func(t *testing.T) {
		w := ClientIPOrderWarnings("entrypoint", []map[string]any{redirect, cfRealIP, whitelist})
		expect.Equal(t, len(w), 0)
	})

	t.Run("no_real_ip_middleware", func(t *testing.T) {
		w := ClientIPOrderWarnings("entrypoint", []map[string]any{redirect, whitelist})
		expect.Equal(t, len(w), 0)
	})

	t.Run("case_and_snake_insensitive", func(t *testing.T) {
		w := ClientIPOrderWarnings("entrypoint", []map[string]any{
			{"use": "RateLimit", "average": 10, "burst": 10},
			{"use": "realIP", "header": "X-Real-IP", "from": []string{"10.0.0.0/8"}},
		})
		expect.Equal(t, len(w), 1)
	})

	t.Run("bypass_with_remote", func(t *testing.T) {
		w := ClientIPOrderWarnings("entrypoint", []map[string]any{
			{"use": "redirect_http", "bypass": []string{"remote 192.168.0.0/16"}},
			cfRealIP,
		})
		expect.Equal(t, len(w), 1)
		expect.True(t, strings.Contains(w[0], "bypass rules matching `remote`"))
	})

	t.Run("bypass_without_remote", func(t *testing.T) {
		w := ClientIPOrderWarnings("entrypoint", []map[string]any{
			{"use": "redirect_http", "bypass": []string{"route hass", "path /remote/*"}},
			cfRealIP,
		})
		expect.Equal(t, len(w), 0)
	})
}

func TestClientIPOrderWarningsReaderBetweenResolvers(t *testing.T) {
	w := ClientIPOrderWarnings("entrypoint", []map[string]any{
		{"use": "real_ip", "header": "X-Real-IP", "from": []string{"10.0.0.0/8"}},
		{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}},
		{"use": "cloudflare_real_ip"},
	})
	expect.Equal(t, len(w), 1)
	expect.True(t, strings.Contains(w[0], "entrypoint[1] cidr_whitelist reads the client IP"))
	expect.True(t, strings.Contains(w[0], "entrypoint[2] cloudflare_real_ip"))
}

// registerComposeChain makes a compose chain available as name@file for a test.
func registerComposeChain(t *testing.T, name string, defs []map[string]any) string {
	t.Helper()
	chain, err := BuildMiddlewareFromChainRaw(name, defs)
	expect.NoError(t, err)
	key := strutils.ToLowerNoSnake(name + "@file")
	allMiddlewares[key] = chain
	t.Cleanup(func() { delete(allMiddlewares, key) })
	return name + "@file"
}

func TestClientIPOrderWarningsComposeChains(t *testing.T) {
	guard := registerComposeChain(t, "guard", []map[string]any{
		{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}},
	})
	resolve := registerComposeChain(t, "resolve", []map[string]any{
		{"use": "cloudflare_real_ip"},
		{"use": "rate_limit", "average": 10, "burst": 10},
	})

	w := ClientIPOrderWarnings("entrypoint", []map[string]any{{"use": guard}, {"use": "cloudflare_real_ip"}})
	expect.Equal(t, len(w), 1, "a compose chain that reads the client IP counts as a reader")

	w = ClientIPOrderWarnings("entrypoint", []map[string]any{{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}}, {"use": resolve}})
	expect.Equal(t, len(w), 1, "a compose chain that starts by resolving counts as a resolver")

	w = ClientIPOrderWarnings("entrypoint", []map[string]any{{"use": resolve}, {"use": guard}})
	expect.Equal(t, len(w), 0)

	remoteBypass := registerComposeChain(t, "remotebypass", []map[string]any{
		{"use": "redirect_http", "bypass": []string{"remote 127.0.0.1"}},
	})
	w = ClientIPOrderWarnings("entrypoint", []map[string]any{{"use": remoteBypass}, {"use": "cloudflare_real_ip"}})
	expect.Equal(t, len(w), 1, "a compose chain member with a remote bypass reads the client IP")
}

func TestPromotedClientIPOrderWarnings(t *testing.T) {
	base := []map[string]any{
		{"use": "redirect_http"},
		{"use": "cloudflare_real_ip"},
		{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}},
	}
	effective := cloneMiddlewareDefs(base)
	effective[0]["bypass"] = []string{"route plex & remote 203.0.113.7"}
	effective[2]["bypass"] = []string{"route plex & remote 203.0.113.7"}

	w := promotedClientIPOrderWarnings("entrypoint", base, effective)
	expect.Equal(t, len(w), 1, "only the bypass promoted before cloudflare_real_ip is new")
	expect.True(t, strings.Contains(w[0], "entrypoint[0] redirect_http has bypass rules matching `remote`"))
}

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

	for _, tc := range []struct {
		rule string
		want int
	}{
		{`"remote" 127.0.0.1`, 1},
		{`path /health | !remote 127.0.0.1`, 1},
		{`header X-Note "please remote 127.0.0.1"`, 0},
		{`header X-Note "!remote 127.0.0.1" & path /health`, 0},
	} {
		t.Run(tc.rule, func(t *testing.T) {
			defs := []map[string]any{{"use": "redirect_http", "bypass": []string{tc.rule}}, cfRealIP}
			_, err := BuildMiddlewareFromChainRaw("parsed", defs)
			expect.NoError(t, err)
			expect.Equal(t, len(ClientIPOrderWarnings("parsed", defs)), tc.want)
		})
	}
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

func TestClientIPOrderWarningsBypassRequestPhase(t *testing.T) {
	for _, tc := range []struct {
		use  string
		want int
	}{
		{"modify_response", 0},
		{"modify_request", 1},
	} {
		t.Run(tc.use, func(t *testing.T) {
			def := map[string]any{
				"use": tc.use, "set_headers": map[string]string{"X-Phase": "applied"},
				"bypass": []string{"remote 127.0.0.1"},
			}
			resolver := map[string]any{"use": "real_ip", "from": []string{"127.0.0.1"}}
			expect.Equal(t, len(ClientIPOrderWarnings("raw", []map[string]any{def, resolver})), tc.want)
			nested := registerComposeChain(t, "phase", []map[string]any{def})
			expect.Equal(t, len(ClientIPOrderWarnings("nested", []map[string]any{{"use": nested}, resolver})), tc.want)
			// Response-only bypasses must not hide the resolver preference of
			// a compose chain that otherwise starts by resolving the client IP.
			chain, err := BuildMiddlewareFromChainRaw("sorted", []map[string]any{{"use": nested}, resolver})
			expect.NoError(t, err)
			expect.Equal(t, isClientIPResolver(chain.impl), tc.want == 0)
		})
	}
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

	mixed := registerComposeChain(t, "mixed", []map[string]any{
		{"use": guard},
		{"use": "cloudflare_real_ip"},
	})
	w = ClientIPOrderWarnings("entrypoint", []map[string]any{{"use": guard}, {"use": mixed}})
	expect.Equal(t, len(w), 1, "a nested reader must not hide its later resolver from outer diagnostics")
	w = ClientIPOrderWarnings("entrypoint", []map[string]any{{"use": mixed}, {"use": "cloudflare_real_ip"}})
	expect.Equal(t, len(w), 1, "a nested resolver must not hide readers from outer diagnostics")
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

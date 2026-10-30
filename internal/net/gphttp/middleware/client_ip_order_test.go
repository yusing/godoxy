package middleware

import (
	"strings"
	"testing"

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

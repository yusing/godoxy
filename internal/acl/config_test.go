package acl

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIPAllowedCachesDecision(t *testing.T) {
	t.Parallel()

	testIP := net.ParseIP("8.8.8.8")
	require.NotNil(t, testIP)

	t.Run("cached allow survives rule changes", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			Default:    ACLDeny,
			AllowLocal: new(false),
			Allow:      mustMatchers(t, "ip:8.8.8.8"),
		}
		require.NoError(t, cfg.Validate())

		require.True(t, cfg.IPAllowed(testIP))

		cfg.Allow = nil
		cfg.Deny = mustMatchers(t, "ip:8.8.8.8")

		require.True(t, cfg.IPAllowed(testIP))
	})

	t.Run("cached deny survives rule changes", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{
			Default:    ACLAllow,
			AllowLocal: new(false),
			Deny:       mustMatchers(t, "ip:8.8.8.8"),
		}
		require.NoError(t, cfg.Validate())

		require.False(t, cfg.IPAllowed(testIP))

		cfg.Deny = nil
		cfg.Allow = mustMatchers(t, "ip:8.8.8.8")

		require.False(t, cfg.IPAllowed(testIP))
	})
}

func mustMatchers(t *testing.T, rules ...string) Matchers {
	t.Helper()

	matchers := make(Matchers, len(rules))
	for i, rule := range rules {
		require.NoError(t, matchers[i].Parse(rule))
	}
	return matchers
}

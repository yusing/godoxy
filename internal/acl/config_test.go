package acl

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/maxmind"
	maxmindtypes "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/goutils/task"
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

func TestCountryACLWithDatabase(t *testing.T) {
	db, err := os.ReadFile("testdata/GeoIP2-Country-Test.mmdb")
	require.NoError(t, err)
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir("data", 0o700))
	require.NoError(t, os.WriteFile(filepath.Join("data", "GeoLite2-Country.mmdb"), db, 0o600))
	parent := task.GetTestTask(t)
	instance, err := maxmind.New(parent, &maxmind.Config{Database: maxmindtypes.MaxMindGeoLite})
	require.NoError(t, err)
	maxmind.SetCtx(parent, instance)

	for _, tt := range []struct {
		name   string
		ip     string
		allow  []string
		deny   []string
		want   bool
		reason string
	}{
		{"country first", "81.2.69.160", []string{"country:GB", "tz:Asia/Shanghai"}, nil, true, "allowed by allow rule: country:GB"},
		{"country after country miss", "81.2.69.160", []string{"country:CN", "country:GB"}, nil, true, "allowed by allow rule: country:GB"},
		{"country after timezone miss", "81.2.69.160", []string{"tz:Asia/Shanghai", "country:GB"}, nil, true, "allowed by allow rule: country:GB"},
		{"country after deny miss", "81.2.69.160", []string{"country:GB"}, []string{"country:CN"}, true, "allowed by allow rule: country:GB"},
		{"later deny wins", "81.2.69.160", []string{"country:GB"}, []string{"country:CN", "country:GB"}, false, "blocked by deny rule: country:GB"},
		{"deny wins", "81.2.69.160", []string{"country:GB"}, []string{"country:GB"}, false, "blocked by deny rule: country:GB"},
		{"unmatched country", "175.16.199.1", []string{"country:GB"}, nil, false, "denied by default"},
		{"country database has no timezone", "175.16.199.1", []string{"tz:Asia/Shanghai"}, nil, false, "denied by default"},
		{"IPv6 later country", "2001:218::1", []string{"country:GB", "country:JP"}, nil, true, "allowed by allow rule: country:JP"},
		{"unknown IP", "192.0.2.1", []string{"country:GB"}, nil, false, "denied by default"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Default: ACLDeny, Allow: mustMatchers(t, tt.allow...), Deny: mustMatchers(t, tt.deny...)}
			require.NoError(t, cfg.Validate())
			require.NoError(t, cfg.Start(parent))
			t.Cleanup(cfg.notifyTicker.Stop)
			for range 2 { // Exercise both the initial decision and the ACL cache hit.
				require.Equal(t, tt.want, cfg.IPAllowed(net.ParseIP(tt.ip)))
				record, err := cfg.ipCache(parent.Context(), tt.ip)
				require.NoError(t, err)
				require.Equal(t, tt.reason, record.reason)
			}
		})
	}
}

func TestCountryACLWithoutDatabase(t *testing.T) {
	for _, configured := range []bool{false, true} {
		name := "missing context instance"
		if configured {
			name = "database not loaded"
		}
		t.Run(name, func(t *testing.T) {
			parent := task.GetTestTask(t)
			if configured {
				instance, err := maxmind.New(parent, &maxmind.Config{})
				require.NoError(t, err)
				maxmind.SetCtx(parent, instance)
			}
			cfg := &Config{Default: ACLDeny, Allow: mustMatchers(t, "country:BR", "tz:Asia/Shanghai")}
			require.NoError(t, cfg.Validate())
			require.NoError(t, cfg.Start(parent))
			t.Cleanup(cfg.notifyTicker.Stop)
			require.False(t, cfg.IPAllowed(net.ParseIP("200.1.2.3")))
		})
	}
}

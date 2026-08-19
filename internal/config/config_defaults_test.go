package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	runtimeconfig "github.com/yusing/godoxy/internal/config"
	configtypes "github.com/yusing/godoxy/internal/config/types"
)

// A config file only carries overrides. Every value it leaves out must keep its
// default, otherwise settings such as timeout_shutdown silently become zero and
// shutdown gets no grace period at all.
func TestInitFromFileKeepsDefaults(t *testing.T) {
	defaults := configtypes.DefaultConfig()

	tests := []struct {
		name    string
		content string
		write   bool
	}{
		{name: "file omits the keys", content: "webui:\n  display_name: test\n", write: true},
		{name: "empty file", content: "", write: true},
		{name: "comments only", content: "# no overrides\n", write: true},
		{name: "missing file", write: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			if tc.write {
				require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o644))
			}

			state := runtimeconfig.NewState()
			t.Cleanup(func() { state.Stop(nil) })
			require.NoError(t, state.InitFromFile(path))

			require.Equal(t, defaults.TimeoutShutdown, state.Value().TimeoutShutdown)
			require.Equal(t, defaults.Homepage.UseDefaultCategories, state.Value().Homepage.UseDefaultCategories)
		})
	}
}

func TestInitFromFileAppliesOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("timeout_shutdown: 7\nhomepage:\n  use_default_categories: false\n"), 0o644))

	state := runtimeconfig.NewState()
	t.Cleanup(func() { state.Stop(nil) })
	require.NoError(t, state.InitFromFile(path))

	require.Equal(t, 7, state.Value().TimeoutShutdown)
	require.False(t, state.Value().Homepage.UseDefaultCategories)
}

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/route/rules"
)

func TestLoadWebUIRulesAppendsExtraRules(t *testing.T) {
	extra := rules.Rules{}
	require.NoError(t, extra.Parse(`
- name: extra
  on: path /extra
  do: pass
`))

	combined, err := loadWebUIRules("webui.yml", extra)
	require.NoError(t, err)
	require.Len(t, combined, 5)
	require.Equal(t, "path /login", combined[0].On.String())
	require.Equal(t, "path /extra", combined[len(combined)-1].On.String())
}

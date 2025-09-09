package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigEnvSubstitution(t *testing.T) {
	os.Setenv("CLOUDFLARE_AUTH_TOKEN", "test")
	readFile = func(_ string) ([]byte, error) {
		return []byte(`
---
autocert:
	email: "test@test.com"
	domains:
		- "*.test.com"
	provider: cloudflare
	options:
		auth_token: ${CLOUDFLARE_AUTH_TOKEN}
`), nil
	}

	var cfg Config
	out, err := cfg.readConfigFile()
	require.NoError(t, err)
	require.Equal(t, `
---
autocert:
	email: "test@test.com"
	domains:
		- "*.test.com"
	provider: cloudflare
	options:
		auth_token: "test"
`, string(out))
}

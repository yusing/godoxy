// Package testcert provides local certificate fixtures for tests.
package testcert

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	agentcert "github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/autocert"
)

// WriteFiles writes a certificate and key at the default paths under workDir.
func WriteFiles(tb testing.TB, workDir string) *autocert.Config {
	tb.Helper()
	_, cert, _, err := agentcert.NewAgent()
	require.NoError(tb, err)
	cfg := &autocert.Config{
		Provider: autocert.ProviderLocal,
		CertPath: filepath.Join(workDir, autocert.CertFileDefault),
		KeyPath:  filepath.Join(workDir, autocert.KeyFileDefault),
	}
	require.NoError(tb, os.MkdirAll(filepath.Dir(cfg.CertPath), 0o700))
	require.NoError(tb, os.WriteFile(cfg.CertPath, cert.Cert, 0o600))
	require.NoError(tb, os.WriteFile(cfg.KeyPath, cert.Key, 0o600))
	return cfg
}

// NewProvider returns a provider backed by temporary certificate files.
func NewProvider(tb testing.TB) *autocert.Provider {
	tb.Helper()
	cfg := WriteFiles(tb, tb.TempDir())
	provider, err := autocert.NewProvider(cfg, nil, nil)
	require.NoError(tb, err)
	require.NoError(tb, provider.ObtainCertIfNotExistsAll(tb.Context()))
	return provider
}

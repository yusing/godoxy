package config

import (
	"context"
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
	autocerttypes "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/goutils/task"
)

type stubAutoCertProvider struct{}

func (p *stubAutoCertProvider) GetCert(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return nil, nil
}
func (p *stubAutoCertProvider) GetCertInfos() ([]autocerttypes.CertInfo, error) { return nil, nil }
func (p *stubAutoCertProvider) ScheduleRenewalAll(task.Parent)                  {}
func (p *stubAutoCertProvider) ObtainCertAll() error                            { return nil }
func (p *stubAutoCertProvider) ForceExpiryAll() bool                            { return true }
func (p *stubAutoCertProvider) WaitRenewalDone(context.Context) bool            { return true }

func TestInitAutoCertUsesServiceFactory(t *testing.T) {
	prev := newAutoCertService
	defer func() { newAutoCertService = prev }()

	called := false
	newAutoCertService = func(parent task.Parent, cfg *autocert.Config) (autocerttypes.Provider, error) {
		called = true
		require.NotNil(t, parent)
		require.Equal(t, autocert.ProviderLocal, cfg.Provider)
		return &stubAutoCertProvider{}, nil
	}

	state := NewState().(*state)
	state.AutoCert = &autocert.Config{Provider: autocert.ProviderLocal}

	err := state.initAutoCert()
	require.NoError(t, err)
	require.True(t, called)
	require.NotNil(t, state.autocertProvider)
}

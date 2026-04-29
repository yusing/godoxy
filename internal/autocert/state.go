package autocert

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/yusing/godoxy/internal/common"
	strutils "github.com/yusing/goutils/strings"
)

type CertState int

const (
	CertStateValid CertState = iota
	CertStateExpired
	CertStateMismatch
)

type noopChallengeProvider struct{}

func (noopChallengeProvider) Present(string, string, string) error {
	return nil
}

func (noopChallengeProvider) CleanUp(string, string, string) error {
	return nil
}

var (
	useLocalAutocertOperations atomic.Bool
	autocertCommandRunner      = runAutocertCommand
)

func registerPlaceholderProviders(providerNames ...string) {
	for _, providerName := range providerNames {
		if _, ok := Providers[providerName]; ok {
			continue
		}
		Providers[providerName] = noopChallengeProviderGenerator
	}
}

func noopChallengeProviderGenerator(map[string]strutils.Redacted) (challenge.Provider, error) {
	return noopChallengeProvider{}, nil
}

func UseLocalAutocertOperations() {
	useLocalAutocertOperations.Store(true)
}

func shouldUseLocalAutocertOperations(provider string) bool {
	return useLocalAutocertOperations.Load() || provider == ProviderLocal || provider == ProviderPseudo
}

func obtainCertUsingBinary(ctx context.Context, certPath string) error {
	err := autocertCommandRunner(
		ctx,
		resolveAutocertBinary(),
		"obtain",
		"--config",
		common.ConfigPath,
		"--cert-path",
		certPath,
	)
	if err == nil || !errors.Is(err, exec.ErrNotFound) {
		return err
	}
	return errors.New("autocert binary not found; place autocert next to main binary or on PATH")
}

func resolveAutocertBinary() string {
	if exePath, err := os.Executable(); err == nil {
		siblingPath := filepath.Join(filepath.Dir(exePath), "autocert")
		if _, err := os.Stat(siblingPath); err == nil {
			return siblingPath
		}
	}
	return "autocert"
}

func runAutocertCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

package autocert

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/stretchr/testify/require"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
)

type stubRunner struct {
	mu    sync.Mutex
	modes []RunMode
	err   error
	block chan struct{}
}

func (r *stubRunner) Run(_ context.Context, mode RunMode, _ string) error {
	r.mu.Lock()
	r.modes = append(r.modes, mode)
	block := r.block
	err := r.err
	r.mu.Unlock()
	if block != nil {
		<-block
	}
	return err
}

func (r *stubRunner) calls() []RunMode {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]RunMode(nil), r.modes...)
}

func newTestService(t *testing.T, runner runOnce) *Service {
	t.Helper()
	cache, err := NewFileCache(&Config{Provider: ProviderLocal})
	require.NoError(t, err)
	return &Service{
		cache:        cache,
		runner:       runner,
		cfg:          &Config{Provider: ProviderCustom},
		snapshotPath: filepath.Join(t.TempDir(), "config.yml"),
	}
}

func TestServiceForceExpiryRunsRenewAll(t *testing.T) {
	runner := &stubRunner{}
	svc := newTestService(t, runner)

	ok := svc.ForceExpiryAll()
	require.True(t, ok)
	require.Eventually(t, func() bool { return len(runner.calls()) == 1 }, time.Second, 25*time.Millisecond)
	require.Equal(t, []RunMode{RunModeRenewAll}, runner.calls())
}

func TestServiceRejectsSecondForceExpiryWhileRunning(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	runner := runnerFunc(func(_ context.Context, mode RunMode, _ string) error {
		require.Equal(t, RunModeRenewAll, mode)
		close(started)
		<-finish
		return nil
	})
	svc := newTestService(t, runner)

	require.True(t, svc.ForceExpiryAll())
	<-started
	require.False(t, svc.ForceExpiryAll())
	close(finish)
}

func TestServiceWaitRenewalDoneTracksCurrentRun(t *testing.T) {
	finish := make(chan struct{})
	runner := runnerFunc(func(_ context.Context, _ RunMode, _ string) error {
		<-finish
		return nil
	})
	svc := newTestService(t, runner)

	require.True(t, svc.ForceExpiryAll())
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(finish)
	}()
	require.True(t, svc.WaitRenewalDone(t.Context()))
}

func TestNewServiceRunsInitialObtainIfMissingAndLoadsCache(t *testing.T) {
	prev := newRunner
	defer func() { newRunner = prev }()
	t.Cleanup(func() { _ = os.RemoveAll(runtimeBasePath) })
	const providerName = "helper-test"
	prevProvider, hadProvider := Providers[providerName]
	Providers[providerName] = func(map[string]strutils.Redacted) (challenge.Provider, error) {
		return nil, nil
	}
	t.Cleanup(func() {
		if hadProvider {
			Providers[providerName] = prevProvider
			return
		}
		delete(Providers, providerName)
	})

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.crt")
	keyPath := filepath.Join(dir, "cert.key")
	runner := runnerFunc(func(_ context.Context, mode RunMode, _ string) error {
		require.Equal(t, RunModeObtainIfMissing, mode)
		writeSelfSignedCertToPaths(t, certPath, keyPath, []string{"api.example.com"})
		return nil
	})
	newRunner = func(string) runOnce { return runner }

	root := task.RootTask("test", false)
	t.Cleanup(func() { root.Finish(nil) })

	svc, err := NewService(root, &Config{
		Provider: providerName,
		Email:    "test@example.com",
		Domains:  []string{"api.example.com"},
		CertPath: certPath,
		KeyPath:  keyPath,
	}, "/tmp/helper")
	require.NoError(t, err)

	infos, err := svc.GetCertInfos()
	require.NoError(t, err)
	require.Len(t, infos, 1)
	require.Equal(t, "api.example.com", infos[0].Subject)
}

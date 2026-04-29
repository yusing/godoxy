package autocert

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type stubCmd struct {
	started bool
	waitErr error
}

func (c *stubCmd) Start() error {
	c.started = true
	return nil
}

func (c *stubCmd) Wait() error { return c.waitErr }

type stubFactory struct {
	mode RunMode
	cfg  string
	cmd  *stubCmd
}

func (f *stubFactory) New(_ context.Context, mode RunMode, cfgPath string) oneshotCmd {
	f.mode = mode
	f.cfg = cfgPath
	return f.cmd
}

func TestRunnerRunPassesModeAndSnapshot(t *testing.T) {
	factory := &stubFactory{cmd: &stubCmd{}}
	runner := &Runner{factory: factory}

	err := runner.Run(t.Context(), RunModeRenewAll, "/tmp/autocert.yml")
	require.NoError(t, err)
	require.Equal(t, RunModeRenewAll, factory.mode)
	require.Equal(t, "/tmp/autocert.yml", factory.cfg)
	require.True(t, factory.cmd.started)
}

func TestRunnerRejectsConcurrentRuns(t *testing.T) {
	factory := &stubFactory{cmd: &stubCmd{}}
	runner := &Runner{factory: factory}
	runner.running.Store(true)

	err := runner.Run(t.Context(), RunModeRenewAll, "/tmp/autocert.yml")
	require.ErrorIs(t, err, ErrRunnerBusy)
	require.ErrorContains(t, err, "autocert already running")
}

func TestRunnerPropagatesWaitError(t *testing.T) {
	factory := &stubFactory{cmd: &stubCmd{waitErr: errors.New("boom")}}
	runner := &Runner{factory: factory}

	err := runner.Run(t.Context(), RunModeObtainIfMissing, "/tmp/autocert.yml")
	require.ErrorContains(t, err, "boom")
}

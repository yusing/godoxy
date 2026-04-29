package autocert

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"

	"github.com/yusing/godoxy/internal/logging/memlogger"
)

type RunMode string

const (
	RunModeObtainIfMissing RunMode = "obtain-if-missing"
	RunModeRenewAll        RunMode = "renew-all"
)

var ErrRunnerBusy = errors.New("autocert already running")

type oneshotCmd interface {
	Start() error
	Wait() error
}

type cmdFactory interface {
	New(ctx context.Context, mode RunMode, cfgPath string) oneshotCmd
}

type execFactory struct {
	bin string
}

func (f execFactory) New(ctx context.Context, mode RunMode, cfgPath string) oneshotCmd {
	cmd := exec.CommandContext(ctx, f.bin, "--mode", string(mode), "--config", cfgPath)
	out := io.MultiWriter(os.Stderr, memlogger.GetMemLogger())
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd
}

type Runner struct {
	factory cmdFactory
	running atomic.Bool
}

func NewRunner(bin string) *Runner {
	return &Runner{factory: execFactory{bin: bin}}
}

func (r *Runner) Run(ctx context.Context, mode RunMode, cfgPath string) error {
	if r == nil {
		return errors.New("autocert runner is nil")
	}
	if !r.running.CompareAndSwap(false, true) {
		return ErrRunnerBusy
	}
	defer r.running.Store(false)

	cmd := r.factory.New(ctx, mode, cfgPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start autocert helper %q: %w", mode, err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("run autocert helper %q: %w", mode, err)
	}
	return nil
}

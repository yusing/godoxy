package task

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yusing/go-proxy/internal/logging"
	F "github.com/yusing/go-proxy/internal/utils/functional"
)

var ErrProgramExiting = errors.New("program exiting")

var (
	root     = newRoot()
	allTasks = F.NewSet[*Task]()
)

func testCleanup() {
	root = newRoot()
	allTasks.Clear()
}

// RootTask returns a new Task with the given name, derived from the root context.
func RootTask(name string, needFinish ...bool) *Task {
	return root.Subtask(name, needFinish...)
}

func newRoot() *Task {
	t := &Task{
		name:         "root",
		childrenDone: make(chan struct{}),
		finished:     make(chan struct{}),
	}
	t.ctx, t.cancel = context.WithCancelCause(context.Background())
	return t
}

func RootContext() context.Context {
	return root.ctx
}

func RootContextCanceled() <-chan struct{} {
	return root.ctx.Done()
}

func OnProgramExit(about string, fn func()) {
	root.OnFinished(about, fn)
}

// GracefulShutdown waits for all tasks to finish, up to the given timeout.
//
// If the timeout is exceeded, it prints a list of all tasks that were
// still running when the timeout was reached, and their current tree
// of subtasks.
func GracefulShutdown(timeout time.Duration) (err error) {
	go root.Finish(ErrProgramExiting)

	after := time.After(timeout)
	for {
		select {
		case <-root.finished:
			return
		case <-after:
			b, err := json.Marshal(DebugTaskList())
			if err != nil {
				logging.Warn().Err(err).Msg("failed to marshal tasks")
				return context.DeadlineExceeded
			}
			logging.Warn().RawJSON("tasks", b).Msgf("Timeout waiting for these %d tasks to finish", allTasks.Size())
			return context.DeadlineExceeded
		}
	}
}

func WaitExit(shutdownTimeout int) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)
	signal.Notify(sig, syscall.SIGTERM)
	signal.Notify(sig, syscall.SIGHUP)

	// wait for signal
	<-sig

	// gracefully shutdown
	logging.Info().Msg("shutting down")
	_ = GracefulShutdown(time.Second * time.Duration(shutdownTimeout))
}

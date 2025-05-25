package task

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

var ErrProgramExiting = errors.New("program exiting")

// RootTask returns a new Task with the given name, derived from the root context.
//
//go:inline
func RootTask(name string, needFinish bool) *Task {
	return root.Subtask(name, needFinish)
}

func RootContext() context.Context {
	return root.Context()
}

func RootContextCanceled() <-chan struct{} {
	return root.Context().Done()
}

func OnProgramExit(about string, fn func()) {
	root.OnFinished(about, fn)
}

// WaitExit waits for a signal to shutdown the program, and then waits for all tasks to finish, up to the given timeout.
//
// If the timeout is exceeded, it prints a list of all tasks that were
// still running when the timeout was reached, and their current tree
// of subtasks.
func WaitExit(shutdownTimeout int) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)
	signal.Notify(sig, syscall.SIGTERM)
	signal.Notify(sig, syscall.SIGHUP)

	// wait for signal
	<-sig

	// gracefully shutdown
	log.Info().Msg("shutting down")
	if err := gracefulShutdown(time.Second * time.Duration(shutdownTimeout)); err != nil {
		root.reportStucked()
	}
}

// gracefulShutdown waits for all tasks to finish, up to the given timeout.
//
// If the timeout is exceeded, it prints a list of all tasks that were
// still running when the timeout was reached, and their current tree
// of subtasks.
func gracefulShutdown(timeout time.Duration) error {
	root.mu.Lock()
	if root.isCanceled() {
		cause := context.Cause(root.ctx)
		root.mu.Unlock()
		return cause
	}
	root.mu.Unlock()

	root.cancel(ErrProgramExiting)
	ok := waitEmpty(root.children, timeout)
	root.runOnFinishCallbacks()
	if !ok || !root.waitFinish(timeout) {
		return context.DeadlineExceeded
	}
	return nil
}

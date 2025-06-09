package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

var ErrProgramExiting = errors.New("program exiting")

var root *Task

var closedCh = make(chan struct{})

func init() {
	close(closedCh)
	initRoot()
}

func initRoot() {
	ctx, cancel := context.WithCancelCause(context.Background())
	root = &Task{
		name:   "root",
		ctx:    ctx,
		cancel: cancel,
		done:   closedCh,
	}
	root.parent = root
}

func testCleanup() {
	root.cancel(nil)
	initRoot()
}

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
	root.OnCancel(about, fn)
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
	root.Finish(ErrProgramExiting)
	if !root.waitFinish(timeout) {
		return context.DeadlineExceeded
	}
	return nil
}

func invokeWithRecover(cb *Callback) {
	defer func() {
		if err := recover(); err != nil {
			log.Err(fmtCause(err)).Str("callback", cb.about).Msg("panic")
			panicWithDebugStack()
		}
	}()
	cb.fn()
}

//go:inline
func fmtCause(cause any) error {
	switch cause := cause.(type) {
	case nil:
		return nil
	case error:
		return cause
	case string:
		return errors.New(cause)
	default:
		return fmt.Errorf("%v", cause)
	}
}

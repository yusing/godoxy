// This file has the abstract logic of the task system.
//
// The implementation of the task system is in the impl.go file.
package task

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/gperr"
)

type (
	TaskStarter interface {
		// Start starts the object that implements TaskStarter,
		// and returns an error if it fails to start.
		//
		// callerSubtask.Finish must be called when start fails or the object is finished.
		Start(parent Parent) gperr.Error
		Task() *Task
	}
	TaskFinisher interface {
		Finish(reason any)
	}
	Callback struct {
		fn           func()
		about        string
		waitChildren bool
		done         atomic.Bool
	}
	// Task controls objects' lifetime.
	//
	// Objects that uses a Task should implement the TaskStarter and the TaskFinisher interface.
	//
	// Use Task.Finish to stop all subtasks of the Task.
	Task struct {
		name string

		parent    *Task
		children  childrenSet
		callbacks callbacksSet

		cause error

		canceled chan struct{}

		finished atomic.Bool
		mu       sync.Mutex
	}
	Parent interface {
		Context() context.Context
		Subtask(name string, needFinish bool) *Task
		Name() string
		Finish(reason any)
		OnCancel(name string, f func())
	}
)

type (
	childrenSet  = map[*Task]struct{}
	callbacksSet = map[*Callback]struct{}
)

const taskTimeout = 3 * time.Second

func (t *Task) Context() context.Context {
	return t
}

func (t *Task) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (t *Task) Done() <-chan struct{} {
	return t.canceled
}

func (t *Task) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cause == nil {
		return context.Canceled
	}
	return t.cause
}

func (t *Task) Value(_ any) any {
	return nil
}

// FinishCause returns the reason / error that caused the task to be finished.
func (t *Task) FinishCause() error {
	if t.cause == nil || errors.Is(t.cause, context.Canceled) {
		return nil
	}
	return t.cause
}

// OnFinished calls fn when the task is canceled and all subtasks are finished.
//
// It should not be called after Finish is called.
func (t *Task) OnFinished(about string, fn func()) {
	t.addCallback(about, fn, true)
}

// OnCancel calls fn when the task is canceled.
//
// It should not be called after Finish is called.
func (t *Task) OnCancel(about string, fn func()) {
	t.addCallback(about, fn, false)
}

// Finish cancel all subtasks and wait for them to finish,
// then marks the task as finished, with the given reason (if any).
func (t *Task) Finish(reason any) {
	t.mu.Lock()
	if t.cause != nil {
		t.mu.Unlock()
		return
	}
	cause := fmtCause(reason)
	t.setCause(cause)
	// t does not need finish, it shares the canceled channel with its parent
	if t == root || t.canceled != t.parent.canceled {
		close(t.canceled)
	}
	t.mu.Unlock()

	t.finishAndWait()
}

func (t *Task) finishAndWait() {
	defer putTask(t)

	t.finishChildren()
	t.runCallbacks()

	if !t.waitFinish(taskTimeout) {
		t.reportStucked()
	}
	// clear anyway
	clear(t.children)
	clear(t.callbacks)

	if t != root {
		t.parent.removeChild(t)
	}
	logFinished(t)
}

func (t *Task) isFinished() bool {
	return t.finished.Load()
}

// Subtask returns a new subtask with the given name, derived from the parent's context.
//
// This should not be called after Finish is called on the task or its parent task.
func (t *Task) Subtask(name string, needFinish bool) *Task {
	panicIfFinished(t, "Subtask is called")

	child := newTask(name, t, needFinish)

	if needFinish {
		t.addChild(child)
	}

	logStarted(child)
	return child
}

// Name returns the name of the task without parent names.
func (t *Task) Name() string {
	return t.name
}

// String returns the full name of the task.
func (t *Task) String() string {
	if t.parent != nil {
		return t.parent.String() + "." + t.name
	}
	return t.name
}

// MarshalText implements encoding.TextMarshaler.
func (t *Task) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func invokeWithRecover(cb *Callback) {
	defer func() {
		cb.done.Store(true)
		if err := recover(); err != nil {
			log.Err(fmtCause(err)).Str("callback", cb.about).Msg("panic")
			panicWithDebugStack()
		}
	}()
	cb.fn()
}

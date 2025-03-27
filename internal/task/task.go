package task

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/utils/strutils"
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
	}
	// Task controls objects' lifetime.
	//
	// Objects that uses a Task should implement the TaskStarter and the TaskFinisher interface.
	//
	// Use Task.Finish to stop all subtasks of the Task.
	Task struct {
		name string

		parent       *Task
		children     uint32
		childrenDone chan struct{}

		callbacks     map[*Callback]struct{}
		callbacksDone chan struct{}

		finished chan struct{}
		// finishedCalled == 1 Finish has been called
		// but does not mean that the task is finished yet
		// this is used to avoid calling Finish twice
		finishedCalled uint32

		mu sync.Mutex

		ctx    context.Context
		cancel context.CancelCauseFunc
	}
	Parent interface {
		Context() context.Context
		Subtask(name string, needFinish ...bool) *Task
		Name() string
		Finish(reason any)
		OnCancel(name string, f func())
	}
)

const taskTimeout = 3 * time.Second

func (t *Task) Context() context.Context {
	return t.ctx
}

func (t *Task) Finished() <-chan struct{} {
	return t.finished
}

// FinishCause returns the reason / error that caused the task to be finished.
func (t *Task) FinishCause() error {
	return context.Cause(t.ctx)
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
	if atomic.LoadUint32(&t.finishedCalled) == 1 {
		return
	}

	t.mu.Lock()
	if t.finishedCalled == 1 {
		t.mu.Unlock()
		return
	}

	t.finishedCalled = 1
	t.mu.Unlock()

	t.finish(reason)
}

func (t *Task) finish(reason any) {
	t.cancel(fmtCause(reason))
	if !waitWithTimeout(t.childrenDone) {
		logging.Debug().
			Str("task", t.name).
			Strs("subtasks", t.listChildren()).
			Msg("Timeout waiting for subtasks to finish")
	}
	go t.runCallbacks()
	if !waitWithTimeout(t.callbacksDone) {
		logging.Debug().
			Str("task", t.name).
			Strs("callbacks", t.listCallbacks()).
			Msg("Timeout waiting for callbacks to finish")
	}
	close(t.finished)
	if t == root {
		return
	}
	t.parent.subChildCount()
	allTasks.Remove(t)
	logging.Trace().Msg("task " + t.name + " finished")
}

// Subtask returns a new subtask with the given name, derived from the parent's context.
//
// This should not be called after Finish is called.
func (t *Task) Subtask(name string, needFinish ...bool) *Task {
	nf := len(needFinish) == 0 || needFinish[0]

	ctx, cancel := context.WithCancelCause(t.ctx)
	child := &Task{
		parent:   t,
		finished: make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}
	if t != root {
		child.name = t.name + "." + name
	} else {
		child.name = name
	}

	allTasks.Add(child)
	t.addChildCount()

	if !nf {
		go func() {
			<-child.ctx.Done()
			child.Finish(nil)
		}()
	}

	logging.Trace().Msg("task " + child.name + " started")
	return child
}

// Name returns the name of the task without parent names.
func (t *Task) Name() string {
	parts := strutils.SplitRune(t.name, '.')
	return parts[len(parts)-1]
}

// String returns the full name of the task.
func (t *Task) String() string {
	return t.name
}

// MarshalText implements encoding.TextMarshaler.
func (t *Task) MarshalText() ([]byte, error) {
	return []byte(t.name), nil
}

func (t *Task) invokeWithRecover(fn func(), caller string) {
	defer func() {
		if err := recover(); err != nil {
			logging.Error().
				Interface("err", err).
				Msg("panic in task " + t.name + "." + caller)
			if common.IsDebug {
				panic(string(debug.Stack()))
			}
		}
	}()
	fn()
}

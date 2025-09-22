package task

import (
	"context"
	"sync"
	"time"

	"github.com/yusing/godoxy/internal/gperr"
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
		fn    func()
		about string
	}
	// Task controls objects' lifetime.
	//
	// Objects that uses a Task should implement the TaskStarter and the TaskFinisher interface.
	//
	// Use Task.Finish to stop all subtasks of the Task.
	Task struct {
		parent       *Task
		name         string
		ctx          context.Context
		cancel       context.CancelCauseFunc
		done         chan struct{}
		finishCalled bool
		onCancel     *withWg[*Callback]
		onFinish     *withWg[*Callback]
		children     *withWg[*Task]

		mu sync.Mutex
	}
	Parent interface {
		Context() context.Context
		// Subtask returns a new subtask with the given name, derived from the parent's context.
		//
		// This should not be called after Finish is called on the task or its parent task.
		Subtask(name string, needFinish bool) *Task
		Name() string
		Finish(reason any)
		OnCancel(name string, f func())
	}
)

const taskTimeout = 3 * time.Second

func (t *Task) Context() context.Context {
	return t.ctx
}

func (t *Task) Name() string {
	return t.name
}

// String returns the full name of the task.
func (t *Task) String() string {
	return t.fullName()
}

// MarshalText implements encoding.TextMarshaler.
func (t *Task) MarshalText() ([]byte, error) {
	return []byte(t.fullName()), nil
}

// Finish marks the task as finished, with the given reason (if any).
func (t *Task) Finish(reason any) {
	t.finish(reason, false)
}

// FinishCause returns the reason / error that caused the task to be finished.
func (t *Task) FinishCause() error {
	return context.Cause(t.ctx)
}

// FinishAndWait cancel all subtasks and wait for them to finish,
// then marks the task as finished, with the given reason (if any).
func (t *Task) FinishAndWait(reason any) {
	t.finish(reason, true)
}

// OnFinished calls fn when the task is canceled and all subtasks are finished.
//
// It should not be called after Finish is called.
func (t *Task) OnFinished(about string, fn func()) {
	if !t.needFinish() {
		t.OnCancel(about, fn)
		return
	}

	t.mu.Lock()
	if t.onFinish == nil {
		t.onFinish = newWithWg[*Callback]()
		t.mu.Unlock()

		go func() {
			<-t.ctx.Done()
			<-t.done
			for cb := range t.onFinish.Range {
				go func(cb *Callback) {
					invokeWithRecover(cb)
					t.onFinish.Delete(cb)
				}(cb)
			}
		}()
	} else {
		t.mu.Unlock()
	}

	t.onFinish.Add(&Callback{fn: fn, about: about})
}

// OnCancel calls fn when the task is canceled.
//
// It should not be called after Finish is called.
func (t *Task) OnCancel(about string, fn func()) {
	t.mu.Lock()
	if t.onCancel == nil {
		t.onCancel = newWithWg[*Callback]()
		t.mu.Unlock()

		go func() {
			<-t.ctx.Done()
			for cb := range t.onCancel.Range {
				go func(cb *Callback) {
					invokeWithRecover(cb)
					t.onCancel.Delete(cb)
				}(cb)
			}
		}()
	} else {
		t.mu.Unlock()
	}

	t.onCancel.Add(&Callback{fn: fn, about: about})
}

// Subtask returns a new subtask with the given name, derived from the parent's context.
//
// This should not be called after Finish is called on the task or its parent task.
func (t *Task) Subtask(name string, needFinish bool) *Task {
	t.mu.Lock()
	if t.children == nil {
		t.children = newWithWg[*Task]()
		t.mu.Unlock()
	} else {
		t.mu.Unlock()
	}

	child := &Task{
		name:   name,
		parent: t,
	}

	t.children.Add(child)

	child.ctx, child.cancel = context.WithCancelCause(t.ctx)

	if needFinish {
		child.done = make(chan struct{})
	} else {
		child.done = closedCh
		go func() {
			<-child.ctx.Done()
			child.Finish(t.FinishCause())
		}()
	}

	logStarted(child)
	return child
}

func (t *Task) finish(reason any, wait bool) {
	t.mu.Lock()
	if t.finishCalled {
		t.mu.Unlock()
		// wait but not report stucked (again)
		t.waitFinish(taskTimeout)
		return
	}

	t.finishCalled = true
	t.mu.Unlock()

	if t.needFinish() {
		close(t.done)
	}

	t.cancel(fmtCause(reason))
	if wait && !t.waitFinish(taskTimeout) {
		t.reportStucked()
	}
	if t != root {
		t.parent.children.Delete(t)
	}
	logFinished(t)
}

func (t *Task) waitFinish(timeout time.Duration) bool {
	if t.children == nil && t.onCancel == nil && t.onFinish == nil {
		return true
	}
	done := make(chan struct{})
	go func() {
		if t.children != nil {
			t.children.Wait()
		}
		if t.onCancel != nil {
			t.onCancel.Wait()
		}
		if t.onFinish != nil {
			t.onFinish.Wait()
		}
		<-t.done
		close(done)
	}()
	timeoutCh := time.After(timeout)
	select {
	case <-done:
		return true
	case <-timeoutCh:
		return false
	}
}

func (t *Task) fullName() string {
	if t.parent == root {
		return t.name
	}
	return t.parent.fullName() + "." + t.name
}

func (t *Task) needFinish() bool {
	return t.done != closedCh
}

package task

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

var (
	taskPool = make(chan *Task, 100)

	voidTask = &Task{ctx: context.Background()}
	root     = newRoot()

	cancelCtx context.Context
)

func init() {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelCtx = ctx //nolint:fatcontext

	voidTask.parent = root
}

func testCleanup() {
	root = newRoot()
}

func newRoot() *Task {
	return newTask("root", voidTask, true)
}

func noCancel(error) {
	// do nothing
}

//go:inline
func newTask(name string, parent *Task, needFinish bool) *Task {
	var t *Task
	select {
	case t = <-taskPool:
		t.finished.Store(false)
	default:
		t = &Task{}
	}
	t.name = name
	t.parent = parent
	if needFinish {
		t.ctx, t.cancel = context.WithCancelCause(parent.ctx)
	} else {
		t.ctx, t.cancel = parent.ctx, noCancel
	}
	return t
}

//go:inline
func (t *Task) needFinish() bool {
	return t.ctx != t.parent.ctx
}

//go:inline
func (t *Task) isCanceled() bool {
	return t.cancel == nil
}

//go:inline
func putTask(t *Task) {
	select {
	case taskPool <- t:
	default:
		return
	}
}

//go:inline
func (t *Task) addCallback(about string, fn func(), waitSubTasks bool) {
	if !t.needFinish() {
		if waitSubTasks {
			t.parent.addCallback(about, func() {
				if !t.waitFinish(taskTimeout) {
					t.reportStucked()
				}
				fn()
			}, false)
		} else {
			t.parent.addCallback(about, fn, false)
		}
		return
	}

	if !waitSubTasks {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.callbacksOnCancel == nil {
			t.callbacksOnCancel = make(callbacksSet)
			go func() {
				<-t.ctx.Done()
				for c := range t.callbacksOnCancel {
					go func() {
						invokeWithRecover(c)
						t.mu.Lock()
						delete(t.callbacksOnCancel, c)
						t.mu.Unlock()
					}()
				}
			}()
		}
		t.callbacksOnCancel[&Callback{fn: fn, about: about}] = struct{}{}
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isCanceled() {
		log.Panic().
			Str("task", t.String()).
			Str("callback", about).
			Msg("callback added to canceled task")
		return
	}

	if t.callbacksOnFinish == nil {
		t.callbacksOnFinish = make(callbacksSet)
	}
	t.callbacksOnFinish[&Callback{
		fn:    fn,
		about: about,
	}] = struct{}{}
}

//go:inline
func (t *Task) addChild(child *Task) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isCanceled() {
		log.Panic().
			Str("task", t.String()).
			Str("child", child.Name()).
			Msg("child added to canceled task")
		return
	}

	if t.children == nil {
		t.children = make(childrenSet)
	}
	t.children[child] = struct{}{}
}

//go:inline
func (t *Task) removeChild(child *Task) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.children, child)
}

func (t *Task) runOnFinishCallbacks() {
	if len(t.callbacksOnFinish) == 0 {
		return
	}

	for c := range t.callbacksOnFinish {
		go func() {
			invokeWithRecover(c)
			t.mu.Lock()
			delete(t.callbacksOnFinish, c)
			t.mu.Unlock()
		}()
	}
}

func (t *Task) waitFinish(timeout time.Duration) bool {
	// return directly if already finished
	if t.isFinished() {
		return true
	}

	t.mu.Lock()
	children, callbacksOnCancel, callbacksOnFinish := t.children, t.callbacksOnCancel, t.callbacksOnFinish
	t.mu.Unlock()

	ok := true
	if len(children) != 0 {
		ok = waitEmpty(children, timeout)
	}

	if len(callbacksOnCancel) != 0 {
		ok = ok && waitEmpty(callbacksOnCancel, timeout)
	}

	if len(callbacksOnFinish) != 0 {
		ok = ok && waitEmpty(callbacksOnFinish, timeout)
	}

	return ok
}

//go:inline
func waitEmpty[T comparable](set map[T]struct{}, timeout time.Duration) bool {
	if len(set) == 0 {
		return true
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		if len(set) == 0 {
			return true
		}
		select {
		case <-timer.C:
			return false
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
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

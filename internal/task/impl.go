package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	_ "unsafe"
)

var (
	taskPool = make(chan *Task, 100)

	root = newRoot()
)

func testCleanup() {
	root = newRoot()
}

func newRoot() *Task {
	return newTask("root", nil, true)
}

//go:inline
func newTask(name string, parent *Task, needFinish bool) *Task {
	var t *Task
	select {
	case t = <-taskPool:
	default:
		t = &Task{}
	}
	t.name = name
	t.parent = parent
	if needFinish {
		t.canceled = make(chan struct{})
	} else {
		// it will not be nil, because root task always has a canceled channel
		t.canceled = parent.canceled
	}
	return t
}

func putTask(t *Task) {
	select {
	case taskPool <- t:
	default:
		return
	}
}

//go:inline
func (t *Task) setCause(cause error) {
	if cause == nil {
		t.cause = context.Canceled
	} else {
		t.cause = cause
	}
}

//go:inline
func (t *Task) addCallback(about string, fn func(), waitSubTasks bool) {
	t.mu.Lock()
	if t.cause != nil {
		t.mu.Unlock()
		if waitSubTasks {
			waitEmpty(t.children, taskTimeout)
		}
		fn()
		return
	}

	defer t.mu.Unlock()
	if t.callbacks == nil {
		t.callbacks = make(callbacksSet)
	}
	t.callbacks[&Callback{
		fn:           fn,
		about:        about,
		waitChildren: waitSubTasks,
	}] = struct{}{}
}

//go:inline
func (t *Task) addChild(child *Task) {
	t.mu.Lock()
	if t.cause != nil {
		t.mu.Unlock()
		child.Finish(t.FinishCause())
		return
	}

	defer t.mu.Unlock()

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

func (t *Task) finishChildren() {
	t.mu.Lock()
	if len(t.children) == 0 {
		t.mu.Unlock()
		return
	}

	var wg sync.WaitGroup
	for child := range t.children {
		wg.Add(1)
		go func() {
			defer wg.Done()
			child.Finish(t.cause)
		}()
	}

	clear(t.children)
	t.mu.Unlock()
	wg.Wait()
}

func (t *Task) runCallbacks() {
	t.mu.Lock()
	if len(t.callbacks) == 0 {
		t.mu.Unlock()
		return
	}

	var wg sync.WaitGroup
	var needWait bool

	// runs callbacks that does not need wait first
	for c := range t.callbacks {
		if !c.waitChildren {
			wg.Add(1)
			go func() {
				defer wg.Done()
				invokeWithRecover(c)
			}()
		} else {
			needWait = true
		}
	}

	// runs callbacks that need to wait for children
	if needWait {
		waitEmpty(t.children, taskTimeout)
		for c := range t.callbacks {
			if c.waitChildren {
				wg.Add(1)
				go func() {
					defer wg.Done()
					invokeWithRecover(c)
				}()
			}
		}
	}

	clear(t.callbacks)
	t.mu.Unlock()
	wg.Wait()
}

func (t *Task) waitFinish(timeout time.Duration) bool {
	// return directly if already finished
	if t.isFinished() {
		return true
	}

	if len(t.children) == 0 && len(t.callbacks) == 0 {
		return true
	}

	ok := waitEmpty(t.children, timeout) && waitEmpty(t.callbacks, timeout)
	if !ok {
		return false
	}
	t.finished.Store(true)
	return true
}

//go:inline
func waitEmpty[T comparable](set map[T]struct{}, timeout time.Duration) bool {
	if len(set) == 0 {
		return true
	}

	var sema uint32

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
			runtime_Semacquire(&sema)
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

//go:linkname runtime_Semacquire sync.runtime_Semacquire
func runtime_Semacquire(s *uint32)

package task

import (
	"context"
	"sync"
	"testing"
	"time"

	expect "github.com/yusing/godoxy/internal/utils/testing"
)

func testTask() *Task {
	return RootTask("test", true)
}

func TestChildTaskCancellation(t *testing.T) {
	t.Cleanup(testCleanup)

	parent := testTask()
	child := parent.Subtask("", true)

	go func() {
		defer child.Finish(nil)
		for {
			select {
			case <-child.Context().Done():
				return
			default:
				continue
			}
		}
	}()

	parent.Finish(nil) // should also cancel child

	select {
	case <-child.Context().Done():
		expect.ErrorIs(t, context.Canceled, child.Context().Err())
	default:
		t.Fatal("subTask context was not canceled as expected")
	}
}

func TestTaskStuck(t *testing.T) {
	t.Cleanup(testCleanup)
	task := testTask()
	task.OnCancel("second", func() {
		time.Sleep(time.Second)
	})
	done := make(chan struct{})
	go func() {
		task.FinishAndWait(nil)
		close(done)
	}()
	time.Sleep(time.Millisecond * 100)
	select {
	case <-done:
		t.Fatal("task finished unexpectedly")
	default:
	}
	time.Sleep(time.Second)
	select {
	case <-done:
	default:
		t.Fatal("task did not finish")
	}
}

func TestTaskOnCancelOnFinished(t *testing.T) {
	t.Cleanup(testCleanup)
	task := testTask()

	var shouldTrueOnCancel bool
	var shouldTrueOnFinish bool

	task.OnCancel("", func() {
		shouldTrueOnCancel = true
	})
	task.OnFinished("", func() {
		shouldTrueOnFinish = true
	})

	expect.False(t, shouldTrueOnFinish)
	task.FinishAndWait(nil)
	expect.True(t, shouldTrueOnCancel)
	expect.True(t, shouldTrueOnFinish)
}

func TestCommonFlowWithGracefulShutdown(t *testing.T) {
	t.Cleanup(testCleanup)
	task := testTask()

	finished := false

	task.OnFinished("", func() {
		finished = true
	})

	go func() {
		defer task.FinishAndWait(nil)
		for {
			select {
			case <-task.Context().Done():
				return
			default:
				continue
			}
		}
	}()

	expect.NoError(t, gracefulShutdown(1*time.Second))
	expect.True(t, finished)

	expect.ErrorIs(t, ErrProgramExiting, context.Cause(task.Context()))
	expect.ErrorIs(t, context.Canceled, task.Context().Err())
	expect.ErrorIs(t, ErrProgramExiting, task.FinishCause())
}

func TestTimeoutOnGracefulShutdown(t *testing.T) {
	t.Cleanup(testCleanup)
	_ = testTask()

	expect.ErrorIs(t, context.DeadlineExceeded, gracefulShutdown(time.Millisecond))
}

func TestFinishMultipleCalls(t *testing.T) {
	t.Cleanup(testCleanup)
	task := testTask()
	var wg sync.WaitGroup
	n := 20
	for range n {
		wg.Go(func() {
			task.Finish(nil)
		})
	}
	wg.Wait()
}

func BenchmarkTasksNoFinish(b *testing.B) {
	for b.Loop() {
		task := RootTask("", false)
		task.Subtask("", false).Finish(nil)
		task.Finish(nil)
	}
}

func BenchmarkTasksNeedFinish(b *testing.B) {
	for b.Loop() {
		task := testTask()
		task.Subtask("", true).Finish(nil)
		task.Finish(nil)
	}
}

func BenchmarkContextWithCancel(b *testing.B) {
	for b.Loop() {
		task, taskCancel := context.WithCancelCause(b.Context())
		taskCancel(nil)
		<-task.Done()
	}
}

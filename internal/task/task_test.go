package task

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/yusing/go-proxy/internal/utils/testing"
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
		ExpectError(t, context.Canceled, child.Context().Err())
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
		task.Finish(nil)
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

	ExpectFalse(t, shouldTrueOnFinish)
	task.Finish(nil)
	ExpectTrue(t, shouldTrueOnCancel)
	ExpectTrue(t, shouldTrueOnFinish)
}

func TestCommonFlowWithGracefulShutdown(t *testing.T) {
	t.Cleanup(testCleanup)
	task := testTask()

	finished := false

	task.OnFinished("", func() {
		finished = true
	})

	go func() {
		defer task.Finish(nil)
		for {
			select {
			case <-task.Context().Done():
				return
			default:
				continue
			}
		}
	}()

	ExpectNoError(t, gracefulShutdown(1*time.Second))
	time.Sleep(100 * time.Millisecond)
	ExpectTrue(t, finished)

	ExpectTrue(t, root.waitFinish(1*time.Second))
	ExpectError(t, context.Canceled, context.Cause(task.Context()))
	ExpectError(t, ErrProgramExiting, task.Context().Err())
	ExpectError(t, ErrProgramExiting, task.FinishCause())
}

func TestTimeoutOnGracefulShutdown(t *testing.T) {
	t.Cleanup(testCleanup)
	_ = testTask()

	ExpectError(t, context.DeadlineExceeded, gracefulShutdown(time.Millisecond))
}

func TestFinishMultipleCalls(t *testing.T) {
	t.Cleanup(testCleanup)
	task := testTask()
	var wg sync.WaitGroup
	wg.Add(5)
	for range 5 {
		go func() {
			defer wg.Done()
			task.Finish(nil)
		}()
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
		task, taskCancel := context.WithCancel(b.Context())
		taskCancel()
		<-task.Done()
	}
}

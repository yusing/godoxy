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
	child := parent.Subtask("child", false)

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

	parent.cancel(nil) // should also cancel child

	select {
	case <-child.Context().Done():
		ExpectError(t, context.Canceled, child.Context().Err())
	default:
		t.Fatal("subTask context was not canceled as expected")
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

	ExpectNoError(t, GracefulShutdown(1*time.Second))
	ExpectTrue(t, finished)

	<-root.finished
	ExpectError(t, context.Canceled, task.Context().Err())
	ExpectError(t, ErrProgramExiting, task.FinishCause())
}

func TestTimeoutOnGracefulShutdown(t *testing.T) {
	t.Cleanup(testCleanup)
	_ = testTask()

	ExpectError(t, context.DeadlineExceeded, GracefulShutdown(time.Millisecond))
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

func BenchmarkTasks(b *testing.B) {
	for range b.N {
		task := testTask()
		task.Subtask("", true).Finish(nil)
		task.Finish(nil)
	}
}

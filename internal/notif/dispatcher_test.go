package notif

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/goutils/task"
)

func TestNotifySendsToCurrentDispatcher(t *testing.T) {
	disp := &Dispatcher{
		task:  task.GetTestTask(t).Subtask("notification", true),
		logCh: make(chan *LogMessage, 1),
	}
	SetDispatcher(disp)
	t.Cleanup(func() {
		clearDispatcher(disp)
		disp.task.Finish(nil)
	})

	msg := &LogMessage{Title: "test"}
	Notify(msg)

	require.Same(t, msg, <-disp.logCh)
}

func TestNotifyReturnsWhenDispatcherIsCanceled(t *testing.T) {
	disp := &Dispatcher{
		task:  task.GetTestTask(t).Subtask("notification", true),
		logCh: make(chan *LogMessage),
	}
	SetDispatcher(disp)
	t.Cleanup(func() {
		clearDispatcher(disp)
		disp.task.Finish(nil)
	})

	disp.task.Finish(nil)

	done := make(chan struct{})
	go func() {
		Notify(&LogMessage{Title: "test"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Notify blocked after dispatcher cancellation")
	}
}

func TestNotifyReturnsWhenLogChannelIsClosed(t *testing.T) {
	disp := &Dispatcher{
		task:  task.GetTestTask(t).Subtask("notification", true),
		logCh: make(chan *LogMessage),
	}
	SetDispatcher(disp)
	t.Cleanup(func() {
		clearDispatcher(disp)
		disp.task.Finish(nil)
	})

	disp.closeLogCh()

	Notify(&LogMessage{Title: "test"})
}

func TestNotifyUsesCurrentDispatcherWhenPreviousDispatcherIsClosed(t *testing.T) {
	oldDisp := &Dispatcher{
		task:  task.GetTestTask(t).Subtask("old-notification", true),
		logCh: make(chan *LogMessage),
	}
	newDisp := &Dispatcher{
		task:  task.GetTestTask(t).Subtask("new-notification", true),
		logCh: make(chan *LogMessage, 1),
	}
	SetDispatcher(oldDisp)
	t.Cleanup(func() {
		clearDispatcher(newDisp)
		oldDisp.task.Finish(nil)
		newDisp.task.Finish(nil)
	})

	oldDisp.closeLogCh()
	SetDispatcher(newDisp)

	msg := &LogMessage{Title: "test"}
	Notify(msg)

	require.Same(t, msg, <-newDisp.logCh)
}

func TestNotifyDoesNotRaceWithLogChannelClose(t *testing.T) {
	disp := &Dispatcher{
		task:  task.GetTestTask(t).Subtask("notification", true),
		logCh: make(chan *LogMessage, 1),
	}
	SetDispatcher(disp)
	t.Cleanup(func() {
		clearDispatcher(disp)
		disp.task.Finish(nil)
	})

	var wg sync.WaitGroup
	readDone := make(chan struct{})
	go func() {
		for range disp.logCh {
		}
		close(readDone)
	}()
	for range 100 {
		wg.Go(func() {
			Notify(&LogMessage{Title: "test"})
		})
	}
	wg.Go(disp.closeLogCh)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		<-readDone
	case <-time.After(time.Second):
		t.Fatal("Notify raced with log channel close and blocked")
	}
}

func TestStoppedDispatcherDoesNotClearCurrentDispatcher(t *testing.T) {
	oldDisp := &Dispatcher{task: task.GetTestTask(t).Subtask("old-notification", true)}
	newDisp := &Dispatcher{task: task.GetTestTask(t).Subtask("new-notification", true)}
	SetDispatcher(newDisp)
	t.Cleanup(func() {
		clearDispatcher(newDisp)
		oldDisp.task.Finish(nil)
		newDisp.task.Finish(nil)
	})

	clearDispatcher(oldDisp)

	require.Same(t, newDisp, dispatcher.Load())
}

func TestNewDispatcherDoesNotPublish(t *testing.T) {
	oldDisp := &Dispatcher{task: task.GetTestTask(t).Subtask("old-notification", true)}
	SetDispatcher(oldDisp)
	t.Cleanup(func() {
		clearDispatcher(oldDisp)
		oldDisp.task.Finish(nil)
	})

	newDisp := NewDispatcher(task.GetTestTask(t))
	t.Cleanup(func() {
		newDisp.task.Finish(nil)
	})

	require.Same(t, oldDisp, dispatcher.Load())
}

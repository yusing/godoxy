package notif

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"
	"github.com/yusing/goutils/task"
)

type recordingProvider struct {
	ProviderBase
	messages []*LogMessage
}

func (p *recordingProvider) MarshalMessage(msg *LogMessage) ([]byte, error) {
	p.messages = append(p.messages, msg)
	return nil, nil
}

type successfulNotificationTransport struct{}

func (successfulNotificationTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func TestDispatcherSelectsProvidersAtSendTime(t *testing.T) {
	client := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: successfulNotificationTransport{}}
	t.Cleanup(func() { http.DefaultClient = client })

	disp := testDispatcher(t, task.GetTestTask(t), "notification", 0)
	disp.providers = xsync.NewMap[Provider, struct{}]()
	disp.retryMsg = xsync.NewMap[*RetryMessage, struct{}]()
	initial := &recordingProvider{ProviderBase: ProviderBase{Name: "initial", URL: "http://initial.test"}}
	disp.RegisterProvider(&NotificationConfig{Provider: initial})

	broadcast := &LogMessage{Title: "broadcast"}
	targeted := &LogMessage{Title: "targeted", To: []string{"initial"}}
	later := &recordingProvider{ProviderBase: ProviderBase{Name: "later", URL: "http://later.test"}}
	disp.RegisterProvider(&NotificationConfig{Provider: later})

	disp.dispatch(broadcast)
	disp.dispatch(targeted)

	require.Equal(t, []*LogMessage{broadcast, targeted}, initial.messages)
	require.Equal(t, []*LogMessage{broadcast}, later.messages)
	require.Zero(t, disp.retryMsg.Size())
}

func testDispatcher(t *testing.T, parent *task.Task, name string, size int) *Dispatcher {
	t.Helper()
	disp := &Dispatcher{
		task:  parent.Subtask(name, true),
		logCh: make(chan *LogMessage, size),
	}
	t.Cleanup(func() { disp.task.Finish(nil) })
	return disp
}

func TestNotifierFromContextSendsToOwnedDispatcher(t *testing.T) {
	parent := task.GetTestTask(t).Subtask("runtime", true)
	disp := testDispatcher(t, parent, "notification", 1)
	SetCtx(parent, disp)

	msg := &LogMessage{Title: "test"}
	FromCtx(parent.Context()).Notify(msg)

	require.Same(t, msg, <-disp.logCh)
}

func TestNotifierReturnsWhenDispatcherIsCanceled(t *testing.T) {
	parent := task.GetTestTask(t).Subtask("runtime", true)
	disp := testDispatcher(t, parent, "notification", 0)
	SetCtx(parent, disp)
	disp.task.Finish(nil)

	done := make(chan struct{})
	go func() {
		FromCtx(parent.Context()).Notify(&LogMessage{Title: "test"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Notify blocked after dispatcher cancellation")
	}
}

func TestNotifierReturnsWhenLogChannelIsClosed(t *testing.T) {
	parent := task.GetTestTask(t).Subtask("runtime", true)
	disp := testDispatcher(t, parent, "notification", 0)
	SetCtx(parent, disp)
	disp.closeLogCh()

	FromCtx(parent.Context()).Notify(&LogMessage{Title: "test"})
}

func TestNotifierContextsRemainIsolated(t *testing.T) {
	root := task.GetTestTask(t)
	oldRuntime := root.Subtask("old-runtime", true)
	newRuntime := root.Subtask("new-runtime", true)
	oldDisp := testDispatcher(t, oldRuntime, "notification", 1)
	newDisp := testDispatcher(t, newRuntime, "notification", 1)
	SetCtx(oldRuntime, oldDisp)
	SetCtx(newRuntime, newDisp)

	oldMsg := &LogMessage{Title: "old"}
	newMsg := &LogMessage{Title: "new"}
	FromCtx(oldRuntime.Context()).Notify(oldMsg)
	FromCtx(newRuntime.Context()).Notify(newMsg)

	require.Same(t, oldMsg, <-oldDisp.logCh)
	require.Same(t, newMsg, <-newDisp.logCh)
}

func TestNotifierDoesNotRaceWithLogChannelClose(t *testing.T) {
	parent := task.GetTestTask(t).Subtask("runtime", true)
	disp := testDispatcher(t, parent, "notification", 1)
	SetCtx(parent, disp)
	notifier := FromCtx(parent.Context())

	var wg sync.WaitGroup
	readDone := make(chan struct{})
	go func() {
		for range disp.logCh {
		}
		close(readDone)
	}()
	for range 100 {
		wg.Go(func() {
			notifier.Notify(&LogMessage{Title: "test"})
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

func TestNewDispatcherIsNotVisibleOutsideOwningContext(t *testing.T) {
	root := task.GetTestTask(t)
	runtime := root.Subtask("runtime", true)
	disp := NewDispatcher(runtime)
	t.Cleanup(func() { disp.task.Finish(nil) })

	require.IsType(t, noopNotifier{}, FromCtx(runtime.Context()))
	SetCtx(runtime, disp)
	require.Same(t, disp, FromCtx(runtime.Context()))
	require.IsType(t, noopNotifier{}, FromCtx(root.Context()))
}

package logging

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBufferFlushLifecycle(t *testing.T) {
	var output bytes.Buffer
	restoreOutput := replaceDefaultOutput(&output)
	t.Cleanup(restoreOutput)

	var buffered Buffer
	_, err := buffered.Write([]byte("group start\ngroup end\n"))
	require.NoError(t, err)
	require.Empty(t, output.String(), "pending scope leaked into the process log")

	require.NoError(t, buffered.Flush())
	require.Equal(t, "group start\ngroup end\n", output.String())

	_, err = buffered.Write([]byte("late record\n"))
	require.NoError(t, err)
	require.Equal(t, "group start\ngroup end\nlate record\n", output.String())
}

func TestBufferDiscardDropsPendingAndLateRecords(t *testing.T) {
	var output bytes.Buffer
	restoreOutput := replaceDefaultOutput(&output)
	t.Cleanup(restoreOutput)

	var buffered Buffer
	_, err := buffered.Write([]byte("failed reload\n"))
	require.NoError(t, err)
	buffered.Discard()
	_, err = buffered.Write([]byte("late canceled producer\n"))
	require.NoError(t, err)
	require.NoError(t, buffered.Flush())
	require.Empty(t, output.String())
}

func TestBufferFailedFlushRetainsCompleteRecord(t *testing.T) {
	output := new(failOnceWriter)
	restoreOutput := replaceDefaultOutput(output)
	t.Cleanup(restoreOutput)

	var buffered Buffer
	_, err := buffered.Write([]byte("retry me\n"))
	require.NoError(t, err)
	require.ErrorIs(t, buffered.Flush(), errInjectedWrite)
	require.Empty(t, output.String())

	require.NoError(t, buffered.Flush())
	require.Equal(t, "retry me\n", output.String())
}

func TestBufferPartialFlushRetriesOnlyUnwrittenRemainder(t *testing.T) {
	output := &partialOnceWriter{limit: 4}
	restoreOutput := replaceDefaultOutput(output)
	t.Cleanup(restoreOutput)

	var buffered Buffer
	_, err := buffered.Write([]byte("complete record\n"))
	require.NoError(t, err)
	require.ErrorIs(t, buffered.Flush(), errInjectedWrite)
	require.Equal(t, "comp", output.String())

	require.NoError(t, buffered.Flush())
	require.Equal(t, "complete record\n", output.String())
}

func TestBufferRetryDoesNotDuplicateCompletedProcessDestination(t *testing.T) {
	first := new(bytes.Buffer)
	second := new(failOnceWriter)
	restoreOutput := replaceDefaultOutputs(first, second)
	t.Cleanup(restoreOutput)

	var buffered Buffer
	_, err := buffered.Write([]byte("one copy\n"))
	require.NoError(t, err)
	require.ErrorIs(t, buffered.Flush(), errInjectedWrite)
	require.Equal(t, "one copy\n", first.String())
	require.Empty(t, second.String())

	require.NoError(t, buffered.Flush())
	require.Equal(t, "one copy\n", first.String())
	require.Equal(t, "one copy\n", second.String())
}

func TestBufferPassthroughAfterUnrecoverableFlushDoesNotStayPending(t *testing.T) {
	output := &failWriter{err: errInjectedWrite}
	restoreOutput := replaceDefaultOutput(output)
	t.Cleanup(restoreOutput)

	var buffered Buffer
	_, err := buffered.Write([]byte("unflushed\n"))
	require.NoError(t, err)
	require.ErrorIs(t, buffered.Flush(), errInjectedWrite)
	buffered.Passthrough()

	output.err = nil
	_, err = buffered.Write([]byte("late active record\n"))
	require.NoError(t, err)
	require.Equal(t, "late active record\n", output.String())
}

func TestProcessWriterKeepsUnrelatedRecordOutOfMultilineFlush(t *testing.T) {
	target := &splitWriter{
		groupStarted: make(chan struct{}),
		releaseGroup: make(chan struct{}),
	}
	restoreOutput := replaceDefaultOutput(target)
	t.Cleanup(restoreOutput)
	groupWriter := processWriter{}
	unrelatedWriter := processWriter{}

	groupDone := make(chan struct{})
	go func() {
		defer close(groupDone)
		_, _ = groupWriter.Write([]byte("group start\ngroup end\n"))
	}()
	<-target.groupStarted

	unrelatedStarted := make(chan struct{})
	unrelatedDone := make(chan struct{})
	go func() {
		close(unrelatedStarted)
		_, _ = unrelatedWriter.Write([]byte("unrelated\n"))
		close(unrelatedDone)
	}()
	<-unrelatedStarted
	select {
	case <-unrelatedDone:
		t.Fatal("unrelated write completed inside the grouped flush")
	default:
	}

	close(target.releaseGroup)
	<-groupDone
	<-unrelatedDone
	require.Equal(t, "group start\ngroup end\nunrelated\n", target.String())
}

func TestBufferedFlushCannotOvertakeEarlierProcessRecord(t *testing.T) {
	target := &blockingWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	restoreOutput := replaceDefaultOutput(target)
	t.Cleanup(restoreOutput)

	earlierDone := make(chan struct{})
	go func() {
		defer close(earlierDone)
		_, _ = (processWriter{}).Write([]byte("earlier\n"))
	}()
	<-target.started

	var buffered Buffer
	_, err := buffered.Write([]byte("group start\ngroup end\n"))
	require.NoError(t, err)
	flushDone := make(chan error, 1)
	go func() {
		flushDone <- buffered.Flush()
	}()
	select {
	case err := <-flushDone:
		require.NoError(t, err)
		t.Fatal("buffered flush overtook the earlier process record")
	default:
	}

	close(target.release)
	<-earlierDone
	require.NoError(t, <-flushDone)
	require.Equal(t, "earlier\ngroup start\ngroup end\n", target.String())
}

func TestIndependentWriterDoesNotBlockProcessOutput(t *testing.T) {
	processOutput := new(bytes.Buffer)
	restoreOutput := replaceDefaultOutput(processOutput)
	t.Cleanup(restoreOutput)

	independentOutput := &blockingWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	independentDone := make(chan struct{})
	go func() {
		defer close(independentDone)
		_, _ = (&lockedWriter{out: independentOutput}).Write([]byte("independent\n"))
	}()
	<-independentOutput.started

	processDone := make(chan struct{})
	go func() {
		defer close(processDone)
		_, _ = (processWriter{}).Write([]byte("process\n"))
	}()
	select {
	case <-processDone:
	case <-independentDone:
		t.Fatal("independent writer unexpectedly completed before release")
	case <-time.After(time.Second):
		close(independentOutput.release)
		<-independentDone
		t.Fatal("independent writer blocked process output")
	}

	close(independentOutput.release)
	<-independentDone
	require.Equal(t, "process\n", processOutput.String())
}

func replaceDefaultOutput(output io.Writer) func() {
	return replaceDefaultOutputs(output)
}

func replaceDefaultOutputs(outputs ...io.Writer) func() {
	outputMu.Lock()
	previous := defaultOutputs
	defaultOutputs = slices.Clone(outputs)
	outputMu.Unlock()
	return func() {
		outputMu.Lock()
		defaultOutputs = previous
		outputMu.Unlock()
	}
}

var errInjectedWrite = errors.New("injected write failure")

type failOnceWriter struct {
	bytes.Buffer
	fail bool
}

type partialOnceWriter struct {
	bytes.Buffer
	limit int
}

func (w *partialOnceWriter) Write(p []byte) (int, error) {
	if w.limit > 0 {
		limit := min(w.limit, len(p))
		w.limit = 0
		_, _ = w.Buffer.Write(p[:limit])
		return limit, errInjectedWrite
	}
	return w.Buffer.Write(p)
}

type failWriter struct {
	bytes.Buffer
	err error
}

func (w *failWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}

func (w *failOnceWriter) Write(p []byte) (int, error) {
	if !w.fail {
		w.fail = true
		return 0, errInjectedWrite
	}
	return w.Buffer.Write(p)
}

type splitWriter struct {
	mu           sync.Mutex
	buf          bytes.Buffer
	groupStarted chan struct{}
	releaseGroup chan struct{}
}

func (w *splitWriter) Write(p []byte) (int, error) {
	if bytes.HasPrefix(p, []byte("group start")) {
		w.mu.Lock()
		_, _ = w.buf.WriteString("group start\n")
		w.mu.Unlock()
		close(w.groupStarted)
		<-w.releaseGroup
		w.mu.Lock()
		_, _ = w.buf.WriteString("group end\n")
		w.mu.Unlock()
		return len(p), nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *splitWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

type blockingWriter struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *blockingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

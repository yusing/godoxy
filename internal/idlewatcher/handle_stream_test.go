package idlewatcher

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	idlewatchertypes "github.com/yusing/godoxy/internal/idlewatcher/types"
	nettypes "github.com/yusing/godoxy/internal/net/types"
)

func TestWatcherProxyConnDelegatesToWrappedConnProxy(t *testing.T) {
	w := newTestWatcher(t)
	w.stream = &testConnProxyStream{readDone: make(chan struct{})}
	w.state.Store(&containerState{
		status: idlewatchertypes.ContainerStatusRunning,
		ready:  true,
	})
	before := time.Now().Add(-time.Second)
	w.lastReset.Store(before)

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	go w.ProxyConn(t.Context(), serverConn)

	_, err := clientConn.Write([]byte{1})
	require.NoError(t, err)

	select {
	case <-w.stream.(*testConnProxyStream).readDone:
	case <-time.After(time.Second):
		t.Fatal("wrapped conn proxy did not read from accepted connection")
	}
	require.True(t, w.lastReset.Load().After(before))
}

type testConnProxyStream struct {
	readDone chan struct{}
}

var _ nettypes.Stream = (*testConnProxyStream)(nil)
var _ nettypes.ConnProxy = (*testConnProxyStream)(nil)

func (s *testConnProxyStream) ListenAndServe(context.Context, nettypes.HookFunc, nettypes.HookFunc) error {
	return nil
}

func (s *testConnProxyStream) LocalAddr() net.Addr {
	return &net.TCPAddr{}
}

func (s *testConnProxyStream) Close() error {
	return nil
}

func (s *testConnProxyStream) ProxyConn(_ context.Context, conn net.Conn) {
	defer conn.Close()
	var b [1]byte
	_, _ = conn.Read(b[:])
	close(s.readDone)
}

package stream

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUDPUDPStreamRemovesConnectionWhenResponseHandlerExits(t *testing.T) {
	dstConn, peerConn := net.Pipe()
	require.NoError(t, peerConn.Close())

	stream := &UDPUDPStream{
		conns: make(map[string]*udpUDPConn),
	}
	key := "127.0.0.1:12345"
	conn := &udpUDPConn{
		srcAddr: &net.UDPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 12345,
		},
		dstConn: dstConn,
	}
	conn.lastUsed.Store(time.Now())
	stream.conns[key] = conn

	stream.runConnUntilClosed(t.Context(), key, conn)

	stream.mu.Lock()
	_, ok := stream.conns[key]
	stream.mu.Unlock()
	require.False(t, ok)
	require.True(t, conn.closed.Load())
}

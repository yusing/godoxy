package entrypoint

import (
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func BenchmarkSNIRouterHTTPSForward(b *testing.B) {
	ep := NewTestEntrypoint(b, nil)
	listener, err := ep.sni.Listen(b.Context(), reserveSNIListenAddr(b))
	require.NoError(b, err)
	enableSNIBenchmarkSniffing(ep, listener)
	b.Cleanup(func() { require.NoError(b, listener.Close()) })

	b.ReportAllocs()
	for b.Loop() {
		clientConn, serverConn := net.Pipe()
		clientErr := make(chan error, 1)
		go func() {
			client := tls.Client(clientConn, &tls.Config{
				ServerName:         "unmatched.example.test",
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			})
			clientErr <- client.Handshake()
		}()
		go ep.sni.handle(listener.(*sniListener), serverConn)
		accepted, err := listener.Accept()
		require.NoError(b, err)
		_ = accepted.SetReadDeadline(time.Now().Add(time.Second))
		var first [1]byte
		_, err = io.ReadFull(accepted, first[:])
		require.NoError(b, err)
		require.Equal(b, byte(0x16), first[0])
		require.NoError(b, accepted.Close())
		require.NoError(b, clientConn.Close())
		require.Error(b, <-clientErr)
	}
}

func BenchmarkReadClientHelloServerName(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		clientConn, serverConn := net.Pipe()
		clientErr := make(chan error, 1)
		go func() {
			client := tls.Client(clientConn, &tls.Config{
				ServerName:         "bench.example.test",
				InsecureSkipVerify: true,
				MinVersion:         tls.VersionTLS12,
			})
			clientErr <- client.Handshake()
		}()
		serverName, replayConn, err := readClientHelloServerName(serverConn)
		require.NoError(b, err)
		require.Equal(b, "bench.example.test", serverName)
		_ = replayConn.SetReadDeadline(time.Now().Add(time.Second))
		var first [1]byte
		_, err = io.ReadFull(replayConn, first[:])
		require.NoError(b, err)
		require.Equal(b, byte(0x16), first[0])
		require.NoError(b, replayConn.Close())
		require.NoError(b, clientConn.Close())
		require.Error(b, <-clientErr)
	}
}

func enableSNIBenchmarkSniffing(ep *Entrypoint, listener net.Listener) {
	sniListener := listener.(*sniListener)
	sniListener.routes.Store(&sniRouteTable{byKey: map[string]*sniRouteEntry{"configured": {}}})
	sniListener.sniffing.Store(true)
	go ep.sni.accept(sniListener)
}

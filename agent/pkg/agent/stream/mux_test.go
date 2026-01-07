package stream_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/agent/common"
	"github.com/yusing/godoxy/agent/pkg/agent/stream"
)

var errListenerClosed = errors.New("listener closed")

type connQueueListener struct {
	addr      net.Addr
	conns     chan net.Conn
	closed    chan struct{}
	closeOnce sync.Once
}

func newConnQueueListener(addr net.Addr, buffer int) *connQueueListener {
	return &connQueueListener{
		addr:   addr,
		conns:  make(chan net.Conn, buffer),
		closed: make(chan struct{}),
	}
}

func (l *connQueueListener) push(conn net.Conn) error {
	select {
	case <-l.closed:
		_ = conn.Close()
		return errListenerClosed
	case l.conns <- conn:
		return nil
	}
}

func (l *connQueueListener) Accept() (net.Conn, error) {
	conn, ok := <-l.conns
	if !ok {
		return nil, errListenerClosed
	}
	return conn, nil
}

func (l *connQueueListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
		close(l.conns)
	})
	return nil
}

func (l *connQueueListener) Addr() net.Addr { return l.addr }

func TestTLSALPNMux_HTTPAndStreamShareOnePort(t *testing.T) {
	caPEM, srvPEM, clientPEM, err := agent.NewAgent()
	require.NoError(t, err, "generate agent certs")

	caCert, err := caPEM.ToTLSCert()
	require.NoError(t, err, "parse CA cert")
	srvCert, err := srvPEM.ToTLSCert()
	require.NoError(t, err, "parse server cert")
	clientCert, err := clientPEM.ToTLSCert()
	require.NoError(t, err, "parse client cert")

	baseLn, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err, "listen tcp")
	defer baseLn.Close()
	baseAddr := baseLn.Addr().String()

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(caCert.Leaf)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{*srvCert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"http/1.1", stream.StreamALPN},
	}

	httpLn := newConnQueueListener(baseLn.Addr(), 16)
	streamLn := newConnQueueListener(baseLn.Addr(), 16)
	defer httpLn.Close()
	defer streamLn.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// HTTP server
	httpSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})}
	go func() { _ = httpSrv.Serve(httpLn) }()
	defer func() { _ = httpSrv.Shutdown(context.Background()) }()

	// Stream server
	dstAddr, closeDst := startTCPEcho(t)
	defer closeDst()

	tcpStreamSrv := stream.NewTCPServerFromListener(ctx, streamLn)
	go func() { _ = tcpStreamSrv.Start() }()
	defer func() { _ = tcpStreamSrv.Close() }()

	// Mux loop
	go func() {
		for {
			conn, err := baseLn.Accept()
			if err != nil {
				return
			}
			tlsConn := tls.Server(conn, serverTLS)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = tlsConn.Close()
				continue
			}
			if tlsConn.ConnectionState().NegotiatedProtocol == stream.StreamALPN {
				_ = streamLn.push(tlsConn)
			} else {
				_ = httpLn.push(tlsConn)
			}
		}
	}()

	// HTTP client over the same port
	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{*clientCert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"http/1.1"},
		ServerName:   common.CertsDNSName,
	}
	hc, err := tls.Dial("tcp", baseAddr, clientTLS)
	require.NoError(t, err, "dial https")
	defer hc.Close()
	_ = hc.SetDeadline(time.Now().Add(2 * time.Second))
	_, err = hc.Write([]byte("GET / HTTP/1.1\r\nHost: godoxy-agent\r\n\r\n"))
	require.NoError(t, err, "write http request")
	r := bufio.NewReader(hc)
	statusLine, err := r.ReadString('\n')
	require.NoError(t, err, "read status line")
	require.Contains(t, statusLine, "200", "expected 200")

	// Stream client over the same port
	client, err := stream.NewTCPClient(baseAddr, dstAddr, caCert.Leaf, clientCert)
	require.NoError(t, err, "create stream tcp client")
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	msg := []byte("ping over mux")
	_, err = client.Write(msg)
	require.NoError(t, err, "write stream payload")
	buf := make([]byte, len(msg))
	_, err = io.ReadFull(client, buf)
	require.NoError(t, err, "read stream payload")
	require.Equal(t, msg, buf)
}

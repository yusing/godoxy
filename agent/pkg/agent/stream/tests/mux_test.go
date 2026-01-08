package stream_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent/common"
	"github.com/yusing/godoxy/agent/pkg/agent/stream"
)

func TestTLSALPNMux_HTTPAndStreamShareOnePort(t *testing.T) {
	certs := genTestCerts(t)

	baseLn, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err, "listen tcp")
	defer baseLn.Close()
	baseAddr := baseLn.Addr().String()

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(certs.CaCert)

	serverTLS := &tls.Config{
		Certificates: []tls.Certificate{*certs.SrvCert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"http/1.1", stream.StreamALPN},
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	streamSrv := stream.NewTCPServerHandler(ctx)
	defer func() { _ = streamSrv.Close() }()

	tlsLn := tls.NewListener(baseLn, serverTLS)
	defer func() { _ = tlsLn.Close() }()

	// HTTP server
	httpSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}),
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){
			stream.StreamALPN: func(_ *http.Server, conn *tls.Conn, _ http.Handler) {
				streamSrv.ServeConn(conn)
			},
		},
	}
	go func() { _ = httpSrv.Serve(tlsLn) }()
	defer func() { _ = httpSrv.Close() }()

	// Stream destination
	dstAddr, closeDst := startTCPEcho(t)
	defer closeDst()

	// HTTP client over the same port
	clientTLS := &tls.Config{
		Certificates: []tls.Certificate{*certs.ClientCert},
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
	client := NewTCPClient(t, baseAddr, dstAddr, certs)
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

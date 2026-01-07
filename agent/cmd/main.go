package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/agent/stream"
	"github.com/yusing/godoxy/agent/pkg/env"
	"github.com/yusing/godoxy/agent/pkg/handler"
	"github.com/yusing/godoxy/internal/metrics/systeminfo"
	socketproxy "github.com/yusing/godoxy/socketproxy/pkg"
	gperr "github.com/yusing/goutils/errs"
	httpServer "github.com/yusing/goutils/server"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
	"github.com/yusing/goutils/version"
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

func (l *connQueueListener) Addr() net.Addr {
	return l.addr
}

func main() {
	writer := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "01-02 15:04",
	}
	zerolog.TimeFieldFormat = writer.TimeFormat
	log.Logger = zerolog.New(writer).Level(zerolog.InfoLevel).With().Timestamp().Logger()
	ca := &agent.PEMPair{}
	err := ca.Load(env.AgentCACert)
	if err != nil {
		log.Fatal().Err(err).Msg("init CA error")
	}
	caCert, err := ca.ToTLSCert()
	if err != nil {
		log.Fatal().Err(err).Msg("init CA error")
	}

	srv := &agent.PEMPair{}
	srv.Load(env.AgentSSLCert)
	if err != nil {
		log.Fatal().Err(err).Msg("init SSL error")
	}
	srvCert, err := srv.ToTLSCert()
	if err != nil {
		log.Fatal().Err(err).Msg("init SSL error")
	}

	log.Info().Msgf("GoDoxy Agent version %s", version.Get())
	log.Info().Msgf("Agent name: %s", env.AgentName)
	log.Info().Msgf("Agent port: %d", env.AgentPort)
	log.Info().Msgf("Agent runtime: %s", env.Runtime)

	log.Info().Msg(`
Tips:
1. To change the agent name, you can set the AGENT_NAME environment variable.
2. To change the agent port, you can set the AGENT_PORT environment variable.
	`)

	t := task.RootTask("agent", false)

	// One TCP listener on AGENT_PORT, then multiplex by TLS ALPN:
	// - Stream ALPN: route to TCP stream tunnel handler
	// - Otherwise: route to HTTPS API handler
	tcpListener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: env.AgentPort})
	if err != nil {
		gperr.LogFatal("failed to listen on port", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(caCert.Leaf)

	muxTLSConfig := &tls.Config{
		Certificates: []tls.Certificate{*srvCert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
		// Keep HTTP limited to HTTP/1.1 (matching current agent server behavior)
		// and add the stream tunnel ALPN for multiplexing.
		NextProtos: []string{"http/1.1", stream.StreamALPN},
	}
	if env.AgentSkipClientCertCheck {
		muxTLSConfig.ClientAuth = tls.NoClientCert
	}

	httpLn := newConnQueueListener(tcpListener.Addr(), 128)
	streamLn := newConnQueueListener(tcpListener.Addr(), 128)

	httpSrv := &http.Server{
		Handler: handler.NewAgentHandler(),
		BaseContext: func(net.Listener) context.Context {
			return t.Context()
		},
	}
	{
		subtask := t.Subtask("agent-http", true)
		t.OnCancel("stop_http", func() {
			_ = httpSrv.Shutdown(context.Background())
			_ = httpLn.Close()
		})
		go func() {
			err := httpSrv.Serve(httpLn)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error().Err(err).Msg("agent HTTP server stopped with error")
			}
			subtask.Finish(err)
		}()
		log.Info().Int("port", env.AgentPort).Msg("HTTPS API server started")
	}

	{
		tcpServer := stream.NewTCPServerFromListener(t.Context(), streamLn)
		subtask := t.Subtask("agent-stream-tcp", true)
		t.OnCancel("stop_stream_tcp", func() {
			_ = tcpServer.Close()
			_ = streamLn.Close()
		})
		go func() {
			err := tcpServer.Start()
			subtask.Finish(err)
		}()
		log.Info().Int("port", env.AgentPort).Msg("TCP stream server started")
	}

	{
		udpServer := stream.NewUDPServer(t.Context(), &net.UDPAddr{Port: env.AgentPort}, caCert.Leaf, srvCert)
		subtask := t.Subtask("agent-stream-udp", true)
		t.OnCancel("stop_stream_udp", func() {
			_ = udpServer.Close()
		})
		go func() {
			err := udpServer.Start()
			subtask.Finish(err)
		}()
		log.Info().Int("port", env.AgentPort).Msg("UDP stream server started")
	}

	// Accept raw TCP connections, terminate TLS once, and dispatch by ALPN.
	{
		subtask := t.Subtask("agent-tls-mux", true)
		t.OnCancel("stop_mux", func() {
			_ = tcpListener.Close()
			_ = httpLn.Close()
			_ = streamLn.Close()
		})
		go func() {
			defer subtask.Finish(subtask.FinishCause())
			for {
				select {
				case <-t.Context().Done():
					return
				default:
				}

				conn, err := tcpListener.Accept()
				if err != nil {
					if t.Context().Err() != nil {
						return
					}
					log.Error().Err(err).Msg("failed to accept connection")
					continue
				}

				tlsConn := tls.Server(conn, muxTLSConfig)
				if err := tlsConn.HandshakeContext(t.Context()); err != nil {
					_ = tlsConn.Close()
					log.Debug().Err(err).Msg("TLS handshake failed")
					continue
				}

				alpn := tlsConn.ConnectionState().NegotiatedProtocol
				switch alpn {
				case stream.StreamALPN:
					_ = streamLn.push(tlsConn)
				default:
					_ = httpLn.push(tlsConn)
				}
			}
		}()
	}

	if socketproxy.ListenAddr != "" {
		runtime := strutils.Title(string(env.Runtime))

		log.Info().Msgf("%s socket listening on: %s", runtime, socketproxy.ListenAddr)
		opts := httpServer.Options{
			Name:     runtime,
			HTTPAddr: socketproxy.ListenAddr,
			Handler:  socketproxy.NewHandler(),
		}
		httpServer.StartServer(t, opts)
	}

	systeminfo.Poller.Start()

	task.WaitExit(3)
}

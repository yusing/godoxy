package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"os"

	stdlog "log"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/agent/stream"
	"github.com/yusing/godoxy/agent/pkg/env"
	"github.com/yusing/godoxy/agent/pkg/handler"
	"github.com/yusing/godoxy/internal/metrics/systeminfo"
	socketproxy "github.com/yusing/godoxy/socketproxy/pkg"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
	"github.com/yusing/goutils/version"
)

// TODO: support IPv6

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
	// - Stream ALPN: route to TCP stream tunnel handler (via http.Server.TLSNextProto)
	// - Otherwise: route to HTTPS API handler
	tcpListener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: env.AgentPort})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to listen on port")
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

	// TLS listener feeds the HTTP server. ALPN stream connections are intercepted
	// using http.Server.TLSNextProto.
	tlsLn := tls.NewListener(tcpListener, muxTLSConfig)

	streamSrv := stream.NewTCPServerHandler(t.Context())

	httpSrv := &http.Server{
		Handler: handler.NewAgentHandler(),
		BaseContext: func(net.Listener) context.Context {
			return t.Context()
		},
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){
			// When a client negotiates StreamALPN, net/http will call this hook instead
			// of treating the connection as HTTP.
			stream.StreamALPN: func(_ *http.Server, conn *tls.Conn, _ http.Handler) {
				// ServeConn blocks until the tunnel finishes.
				streamSrv.ServeConn(conn)
			},
		},
	}
	{
		subtask := t.Subtask("agent-http", true)
		t.OnCancel("stop_http", func() {
			_ = streamSrv.Close()
			_ = httpSrv.Close()
			_ = tlsLn.Close()
		})
		go func() {
			err := httpSrv.Serve(tlsLn)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error().Err(err).Msg("agent HTTP server stopped with error")
			}
			subtask.Finish(err)
		}()
		log.Info().Int("port", env.AgentPort).Msg("HTTPS API server started (ALPN mux enabled)")
	}
	log.Info().Int("port", env.AgentPort).Msg("TCP stream handler started (via TLSNextProto)")

	{
		udpServer := stream.NewUDPServer(t.Context(), "udp", &net.UDPAddr{Port: env.AgentPort}, caCert.Leaf, srvCert)
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

	if socketproxy.ListenAddr != "" {
		runtime := strutils.Title(string(env.Runtime))

		log.Info().Msgf("%s socket listening on: %s", runtime, socketproxy.ListenAddr)
		l, err := net.Listen("tcp", socketproxy.ListenAddr)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to listen on port")
		}
		errLog := log.Logger.With().Str("level", "error").Str("component", "socketproxy").Logger()
		srv := http.Server{
			Handler: socketproxy.NewHandler(),
			BaseContext: func(net.Listener) context.Context {
				return t.Context()
			},
			ErrorLog: stdlog.New(&errLog, "", 0),
		}
		go func() {
			err := srv.Serve(l)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error().Err(err).Msg("socket proxy server stopped with error")
			}
		}()
	}

	systeminfo.Poller.Start(t)

	task.WaitExit(3)
}

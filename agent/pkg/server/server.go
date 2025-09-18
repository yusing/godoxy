package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/agent/pkg/env"
	"github.com/yusing/go-proxy/agent/pkg/handler"
	"github.com/yusing/go-proxy/internal/net/gphttp/server"
	"github.com/yusing/go-proxy/internal/task"
)

type Options struct {
	CACert, ServerCert *tls.Certificate
	Port               int
}

func StartAgentServer(parent task.Parent, opt Options) {
	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(opt.CACert.Leaf)

	// Configure TLS
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*opt.ServerCert},
		ClientCAs:    caCertPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}

	if env.AgentSkipClientCertCheck {
		tlsConfig.ClientAuth = tls.NoClientCert
	}

	agentServer := &http.Server{
		Addr:      fmt.Sprintf(":%d", opt.Port),
		Handler:   handler.NewAgentHandler(),
		TLSConfig: tlsConfig,
	}

	server.Start(parent, agentServer, server.WithLogger(&log.Logger))
}

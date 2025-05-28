package server

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/acl"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/task"
)

type CertProvider interface {
	GetCert(_ *tls.ClientHelloInfo) (*tls.Certificate, error)
}

type Server struct {
	Name         string
	CertProvider CertProvider
	http         *http.Server
	https        *http.Server
	startTime    time.Time
	acl          *acl.Config

	l zerolog.Logger
}

type Options struct {
	Name         string
	HTTPAddr     string
	HTTPSAddr    string
	CertProvider CertProvider
	Handler      http.Handler
	ACL          *acl.Config
}

type httpServer interface {
	*http.Server | *http3.Server
	Shutdown(ctx context.Context) error
}

func StartServer(parent task.Parent, opt Options) (s *Server) {
	s = NewServer(opt)
	s.Start(parent)
	return s
}

func NewServer(opt Options) (s *Server) {
	var httpSer, httpsSer *http.Server

	logger := log.With().Str("server", opt.Name).Logger()

	certAvailable := false
	if opt.CertProvider != nil {
		_, err := opt.CertProvider.GetCert(nil)
		certAvailable = err == nil
	}

	if opt.HTTPAddr != "" {
		httpSer = &http.Server{
			Addr:    opt.HTTPAddr,
			Handler: opt.Handler,
		}
	}
	if certAvailable && opt.HTTPSAddr != "" {
		httpsSer = &http.Server{
			Addr:    opt.HTTPSAddr,
			Handler: opt.Handler,
			TLSConfig: &tls.Config{
				GetCertificate: opt.CertProvider.GetCert,
				MinVersion:     tls.VersionTLS12,
			},
		}
	}
	return &Server{
		Name:         opt.Name,
		CertProvider: opt.CertProvider,
		http:         httpSer,
		https:        httpsSer,
		l:            logger,
		acl:          opt.ACL,
	}
}

// Start will start the http and https servers.
//
// If both are not set, this does nothing.
//
// Start() is non-blocking.
func (s *Server) Start(parent task.Parent) {
	s.startTime = time.Now()
	subtask := parent.Subtask("server."+s.Name, false)

	if s.https != nil && common.HTTP3Enabled {
		s.https.TLSConfig.NextProtos = []string{http3.NextProtoH3, "h2", "http/1.1"}
		h3 := &http3.Server{
			Addr:      s.https.Addr,
			Handler:   s.https.Handler,
			TLSConfig: http3.ConfigureTLSConfig(s.https.TLSConfig),
		}
		Start(subtask, h3, s.acl, &s.l)
		if s.http != nil {
			s.http.Handler = advertiseHTTP3(s.http.Handler, h3)
		}
		// s.https is not nil (checked above)
		s.https.Handler = advertiseHTTP3(s.https.Handler, h3)
	}

	Start(subtask, s.http, s.acl, &s.l)
	Start(subtask, s.https, s.acl, &s.l)
}

func Start[Server httpServer](parent task.Parent, srv Server, acl *acl.Config, logger *zerolog.Logger) (port int) {
	if srv == nil {
		return
	}

	setDebugLogger(srv, logger)

	proto := proto(srv)
	task := parent.Subtask(proto, true)

	var lc net.ListenConfig
	var serveFunc func() error

	switch srv := any(srv).(type) {
	case *http.Server:
		srv.BaseContext = func(l net.Listener) context.Context {
			return parent.Context()
		}
		l, err := lc.Listen(task.Context(), "tcp", srv.Addr)
		if err != nil {
			HandleError(logger, err, "failed to listen on port")
			return
		}
		port = l.Addr().(*net.TCPAddr).Port
		if srv.TLSConfig != nil {
			l = tls.NewListener(l, srv.TLSConfig)
		}
		if acl != nil {
			l = acl.WrapTCP(l)
		}
		serveFunc = getServeFunc(l, srv.Serve)
		task.OnCancel("stop", func() {
			stop(srv, l, logger)
		})
	case *http3.Server:
		l, err := lc.ListenPacket(task.Context(), "udp", srv.Addr)
		if err != nil {
			HandleError(logger, err, "failed to listen on port")
			return
		}
		port = l.LocalAddr().(*net.UDPAddr).Port
		if acl != nil {
			l = acl.WrapUDP(l)
		}
		serveFunc = getServeFunc(l, srv.Serve)
		task.OnCancel("stop", func() {
			stop(srv, l, logger)
		})
	}
	logStarted(srv, logger)
	go func() {
		err := convertError(serveFunc())
		if err != nil {
			HandleError(logger, err, "failed to serve "+proto+" server")
		}
		task.Finish(err)
	}()
	return port
}

func stop[Server httpServer](srv Server, l io.Closer, logger *zerolog.Logger) {
	if srv == nil {
		return
	}

	proto := proto(srv)

	ctx, cancel := context.WithTimeout(task.RootContext(), 1*time.Second)
	defer cancel()

	if err := convertError(errors.Join(srv.Shutdown(ctx), l.Close())); err != nil {
		HandleError(logger, err, "failed to shutdown "+proto+" server")
	} else {
		logger.Info().Str("proto", proto).Str("addr", addr(srv)).Msg("server stopped")
	}
}

func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}

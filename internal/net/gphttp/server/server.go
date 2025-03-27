package server

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/autocert"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
)

type Server struct {
	Name         string
	CertProvider *autocert.Provider
	http         *http.Server
	https        *http.Server
	startTime    time.Time

	l zerolog.Logger
}

type Options struct {
	Name         string
	HTTPAddr     string
	HTTPSAddr    string
	CertProvider *autocert.Provider
	Handler      http.Handler
}

func StartServer(parent task.Parent, opt Options) (s *Server) {
	s = NewServer(opt)
	s.Start(parent)
	return s
}

func NewServer(opt Options) (s *Server) {
	var httpSer, httpsSer *http.Server

	logger := logging.With().Str("server", opt.Name).Logger()

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
			},
		}
	}
	return &Server{
		Name:         opt.Name,
		CertProvider: opt.CertProvider,
		http:         httpSer,
		https:        httpsSer,
		l:            logger,
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
	Start(subtask, s.http, &s.l)
	Start(subtask, s.https, &s.l)
}

func Start(parent task.Parent, srv *http.Server, logger *zerolog.Logger) {
	if srv == nil {
		return
	}
	srv.BaseContext = func(l net.Listener) context.Context {
		return parent.Context()
	}

	if common.IsDebug {
		srv.ErrorLog = log.New(logger, "", 0)
	}

	var proto string
	if srv.TLSConfig == nil {
		proto = "http"
	} else {
		proto = "https"
	}

	task := parent.Subtask(proto, false)

	var lc net.ListenConfig

	// Serve already closes the listener on return
	l, err := lc.Listen(task.Context(), "tcp", srv.Addr)
	if err != nil {
		HandleError(logger, err, "failed to listen on port")
		return
	}

	task.OnCancel("stop", func() {
		Stop(srv, logger)
	})

	logger.Info().Str("addr", srv.Addr).Msg("server started")

	go func() {
		if srv.TLSConfig == nil {
			err = srv.Serve(l)
		} else {
			err = srv.Serve(tls.NewListener(l, srv.TLSConfig))
		}
		HandleError(logger, err, "failed to serve "+proto+" server")
	}()
}

func Stop(srv *http.Server, logger *zerolog.Logger) {
	if srv == nil {
		return
	}

	var proto string
	if srv.TLSConfig == nil {
		proto = "http"
	} else {
		proto = "https"
	}

	ctx, cancel := context.WithTimeout(task.RootContext(), 3*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		HandleError(logger, err, "failed to shutdown "+proto+" server")
	} else {
		logger.Info().Str("addr", srv.Addr).Msgf("server stopped")
	}
}

func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}

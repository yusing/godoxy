package server

import (
	"log"
	"log/slog"
	"net/http"

	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog"
	slogzerolog "github.com/samber/slog-zerolog/v2"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/net/gphttp"
)

func advertiseHTTP3(handler http.Handler, h3 *http3.Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor < 3 {
			err := h3.SetQUICHeaders(w.Header())
			if err != nil {
				gphttp.ServerError(w, r, err)
				return
			}
		}
		handler.ServeHTTP(w, r)
	})
}

func proto[Server httpServer](srv Server) string {
	var proto string
	switch src := any(srv).(type) {
	case *http.Server:
		if src.TLSConfig == nil {
			proto = "http"
		} else {
			proto = "https"
		}
	case *http3.Server:
		proto = "h3"
	}
	return proto
}

func addr[Server httpServer](srv Server) string {
	var addr string
	switch src := any(srv).(type) {
	case *http.Server:
		addr = src.Addr
	case *http3.Server:
		addr = src.Addr
	}
	return addr
}

func getServeFunc[listener any](l listener, serve func(listener) error) func() error {
	return func() error {
		return serve(l)
	}
}

func setDebugLogger[Server httpServer](srv Server, logger *zerolog.Logger) {
	if !common.IsDebug {
		return
	}
	switch srv := any(srv).(type) {
	case *http.Server:
		srv.ErrorLog = log.New(logger, "", 0)
	case *http3.Server:
		logOpts := slogzerolog.Option{Level: slog.LevelDebug, Logger: logger}
		srv.Logger = slog.New(logOpts.NewZerologHandler())
	}
}

func logStarted[Server httpServer](srv Server, logger *zerolog.Logger) {
	logger.Info().Str("proto", proto(srv)).Str("addr", addr(srv)).Msg("server started")
}

package route

import (
	"context"
	"fmt"
	"net"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/idlewatcher"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/route/stream"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher/health/monitor"
)

// TODO: support stream load balance.
type StreamRoute struct {
	*Route
	stream nettypes.Stream

	l zerolog.Logger
}

func NewStreamRoute(base *Route) (routes.Route, gperr.Error) {
	// TODO: support non-coherent scheme
	return &StreamRoute{
		Route: base,
		l: log.With().
			Str("type", string(base.Scheme)).
			Str("name", base.Name()).
			Logger(),
	}, nil
}

func (r *StreamRoute) Stream() nettypes.Stream {
	return r.stream
}

// Start implements task.TaskStarter.
func (r *StreamRoute) Start(parent task.Parent) gperr.Error {
	stream, err := r.initStream()
	if err != nil {
		return gperr.Wrap(err)
	}
	r.stream = stream

	r.task = parent.Subtask("stream."+r.Name(), !r.ShouldExclude())

	switch {
	case r.UseIdleWatcher():
		waker, err := idlewatcher.NewWatcher(parent, r, r.IdlewatcherConfig())
		if err != nil {
			r.task.Finish(err)
			return gperr.Wrap(err, "idlewatcher error")
		}
		r.stream = waker
		r.HealthMon = waker
	case r.UseHealthCheck():
		r.HealthMon = monitor.NewMonitor(r)
	}

	if r.HealthMon != nil {
		if err := r.HealthMon.Start(r.task); err != nil {
			gperr.LogWarn("health monitor error", err, &r.l)
		}
	}

	if r.ShouldExclude() {
		return nil
	}

	if err := checkExists(r); err != nil {
		return err
	}

	r.ListenAndServe(r.task.Context(), nil, nil)
	r.l = r.l.With().Stringer("rurl", r.ProxyURL).Stringer("laddr", r.LocalAddr()).Logger()
	r.l.Info().Msg("stream started")

	r.task.OnCancel("close_stream", func() {
		r.stream.Close()
		r.l.Info().Msg("stream closed")
	})

	routes.Stream.Add(r)
	r.task.OnCancel("remove_route_from_stream", func() {
		routes.Stream.Del(r)
	})
	return nil
}

func (r *StreamRoute) ListenAndServe(ctx context.Context, preDial, onRead nettypes.HookFunc) {
	r.stream.ListenAndServe(ctx, preDial, onRead)
}

func (r *StreamRoute) Close() error {
	return r.stream.Close()
}

func (r *StreamRoute) LocalAddr() net.Addr {
	return r.stream.LocalAddr()
}

func (r *StreamRoute) initStream() (nettypes.Stream, error) {
	lurl, rurl := r.LisURL, r.ProxyURL
	if lurl != nil && lurl.Scheme != rurl.Scheme {
		return nil, fmt.Errorf("incoherent scheme is not yet supported: %s != %s", lurl.Scheme, rurl.Scheme)
	}

	laddr := ":0"
	if lurl != nil {
		laddr = lurl.Host
	}

	switch rurl.Scheme {
	case "tcp":
		return stream.NewTCPTCPStream(laddr, rurl.Host)
	case "udp":
		return stream.NewUDPUDPStream(laddr, rurl.Host)
	}
	return nil, fmt.Errorf("unknown scheme: %s", rurl.Scheme)
}

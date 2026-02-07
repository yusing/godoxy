package route

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/health/monitor"
	"github.com/yusing/godoxy/internal/idlewatcher"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/route/stream"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/task"
)

// TODO: support stream load balance.
type StreamRoute struct {
	*Route
	stream nettypes.Stream

	l zerolog.Logger
}

func NewStreamRoute(base *Route) (types.Route, gperr.Error) {
	// TODO: support non-coherent scheme
	return &StreamRoute{Route: base}, nil
}

func (r *StreamRoute) Stream() nettypes.Stream {
	return r.stream
}

// Start implements task.TaskStarter.
func (r *StreamRoute) Start(parent task.Parent) gperr.Error {
	if r.LisURL == nil {
		return gperr.Errorf("listen URL is not set")
	}

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
			r.HealthMon = nil
		}
	}

	ep := entrypoint.FromCtx(parent.Context())
	if ep == nil {
		err := gperr.New("entrypoint not initialized")
		r.task.Finish(err)
		return err
	}
	if err := ep.StartAddRoute(r); err != nil {
		r.task.Finish(err)
		return gperr.Wrap(err)
	}
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
	// tcp4/tcp6 -> tcp, udp4/udp6 -> udp
	lScheme := strings.TrimRight(lurl.Scheme, "46")
	rScheme := strings.TrimRight(rurl.Scheme, "46")
	if lScheme != rScheme {
		return nil, fmt.Errorf("incoherent scheme is not yet supported: %s != %s", lurl.Scheme, rurl.Scheme)
	}

	laddr := ":0"
	if lurl != nil {
		laddr = lurl.Host
	}

	switch rScheme {
	case "tcp":
		return stream.NewTCPTCPStream(lurl.Scheme, rurl.Scheme, laddr, rurl.Host, r.GetAgent())
	case "udp":
		return stream.NewUDPUDPStream(lurl.Scheme, rurl.Scheme, laddr, rurl.Host, r.GetAgent())
	}
	return nil, fmt.Errorf("unknown scheme: %s", rurl.Scheme)
}

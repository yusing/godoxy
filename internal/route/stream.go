package route

import (
	"context"
	"errors"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/idlewatcher"
	"github.com/yusing/go-proxy/internal/logging"
	net "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher/health"
	"github.com/yusing/go-proxy/internal/watcher/health/monitor"
)

// TODO: support stream load balance.
type StreamRoute struct {
	*Route

	net.Stream `json:"-"`

	HealthMon health.HealthMonitor `json:"health"`

	task *task.Task

	l zerolog.Logger
}

func NewStreamRoute(base *Route) (routes.Route, gperr.Error) {
	// TODO: support non-coherent scheme
	return &StreamRoute{
		Route: base,
		l: logging.With().
			Str("type", string(base.Scheme)).
			Str("name", base.Name()).
			Logger(),
	}, nil
}

// Start implements task.TaskStarter.
func (r *StreamRoute) Start(parent task.Parent) gperr.Error {
	if existing, ok := routes.Stream.Get(r.Key()); ok {
		return gperr.Errorf("route already exists: from provider %s and %s", existing.ProviderName(), r.ProviderName())
	}
	r.task = parent.Subtask("stream." + r.Name())
	r.Stream = NewStream(r)

	switch {
	case r.UseIdleWatcher():
		waker, err := idlewatcher.NewWatcher(parent, r)
		if err != nil {
			r.task.Finish(err)
			return gperr.Wrap(err, "idlewatcher error")
		}
		r.Stream = waker
		r.HealthMon = waker
	case r.UseHealthCheck():
		r.HealthMon = monitor.NewMonitor(r)
	}

	if err := r.Stream.Setup(); err != nil {
		r.task.Finish(err)
		return gperr.Wrap(err)
	}

	r.l.Info().Int("port", r.Port.Listening).Msg("listening")

	if r.HealthMon != nil {
		if err := r.HealthMon.Start(r.task); err != nil {
			gperr.LogWarn("health monitor error", err, &r.l)
		}
	}

	go r.acceptConnections()

	routes.Stream.Add(r)
	r.task.OnFinished("entrypoint_remove_route", func() {
		routes.Stream.Del(r)
	})
	return nil
}

// Task implements task.TaskStarter.
func (r *StreamRoute) Task() *task.Task {
	return r.task
}

// Finish implements task.TaskFinisher.
func (r *StreamRoute) Finish(reason any) {
	r.task.Finish(reason)
}

func (r *StreamRoute) HealthMonitor() health.HealthMonitor {
	return r.HealthMon
}

func (r *StreamRoute) acceptConnections() {
	defer r.task.Finish("listener closed")

	for {
		select {
		case <-r.task.Context().Done():
			return
		default:
			conn, err := r.Stream.Accept()
			if err != nil {
				select {
				case <-r.task.Context().Done():
				default:
					gperr.LogError("accept connection error", err, &r.l)
				}
				r.task.Finish(err)
				return
			}
			if conn == nil {
				panic("connection is nil")
			}
			go func() {
				err := r.Stream.Handle(conn)
				if err != nil && !errors.Is(err, context.Canceled) {
					gperr.LogError("handle connection error", err, &r.l)
				}
			}()
		}
	}
}

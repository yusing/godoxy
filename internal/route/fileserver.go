package route

import (
	"net/http"
	"path"
	"path/filepath"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging/accesslog"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher/health/monitor"
)

type (
	FileServer struct {
		*Route

		middleware   *middleware.Middleware
		handler      http.Handler
		accessLogger *accesslog.AccessLogger
	}
)

func handler(root string) http.Handler {
	return http.FileServer(http.Dir(root))
}

func NewFileServer(base *Route) (*FileServer, gperr.Error) {
	s := &FileServer{Route: base}

	s.Root = filepath.Clean(s.Root)
	if !path.IsAbs(s.Root) {
		return nil, gperr.New("`root` must be an absolute path")
	}

	s.handler = handler(s.Root)

	if len(s.Middlewares) > 0 {
		mid, err := middleware.BuildMiddlewareFromMap(s.Alias, s.Middlewares)
		if err != nil {
			return nil, err
		}
		s.middleware = mid
	}

	return s, nil
}

// Start implements task.TaskStarter.
func (s *FileServer) Start(parent task.Parent) gperr.Error {
	s.task = parent.Subtask("fileserver."+s.Name(), false)

	pathPatterns := s.PathPatterns
	switch {
	case len(pathPatterns) == 0:
	case len(pathPatterns) == 1 && pathPatterns[0] == "/":
	default:
		mux := gphttp.NewServeMux()
		patErrs := gperr.NewBuilder("invalid path pattern(s)")
		for _, p := range pathPatterns {
			patErrs.Add(mux.Handle(p, s.handler))
		}
		if err := patErrs.Error(); err != nil {
			s.task.Finish(err)
			return err
		}
		s.handler = mux
	}

	if s.middleware != nil {
		s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.middleware.ServeHTTP(s.handler.ServeHTTP, w, r)
		})
	}

	if s.UseAccessLog() {
		var err error
		s.accessLogger, err = accesslog.NewAccessLogger(s.task, s.AccessLog)
		if err != nil {
			s.task.Finish(err)
			return gperr.Wrap(err)
		}
	}

	if s.UseHealthCheck() {
		s.HealthMon = monitor.NewFileServerHealthMonitor(s.HealthCheck, s.Root)
		if err := s.HealthMon.Start(s.task); err != nil {
			return err
		}
	}

	if s.ShouldExclude() {
		return nil
	}

	if err := checkExists(s); err != nil {
		return err
	}

	routes.HTTP.Add(s)
	s.task.OnFinished("remove_route_from_http", func() {
		routes.HTTP.Del(s)
	})
	return nil
}

// ServeHTTP implements http.Handler.
func (s *FileServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.handler.ServeHTTP(w, req)
	if s.accessLogger != nil {
		s.accessLogger.Log(req, req.Response)
	}
}

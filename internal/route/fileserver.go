package route

import (
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/rs/zerolog/log"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/health/monitor"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	gphttp "github.com/yusing/godoxy/internal/net/gphttp"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/task"
)

type (
	FileServer struct {
		*Route

		middleware   *middleware.Middleware
		handler      http.Handler
		accessLogger accesslog.AccessLogger
	}
)

var _ types.FileServerRoute = (*FileServer)(nil)

func handler(root string, spa bool, index string) http.Handler {
	if !spa {
		return http.FileServer(http.Dir(root))
	}
	indexPath := filepath.Join(root, index)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlPath := path.Clean(r.URL.Path)
		if urlPath == "/" {
			http.ServeFile(w, r, indexPath)
			return
		}
		fullPath := filepath.Join(root, filepath.FromSlash(urlPath))
		stat, err := os.Stat(fullPath)
		if err == nil && !stat.IsDir() {
			http.ServeFile(w, r, fullPath)
			return
		}
		http.ServeFile(w, r, indexPath)
	})
}

func NewFileServer(base *Route) (*FileServer, gperr.Error) {
	s := &FileServer{Route: base}

	s.Root = filepath.Clean(s.Root)
	if !path.IsAbs(s.Root) {
		return nil, gperr.New("`root` must be an absolute path")
	}

	if s.Index == "" {
		s.Index = "/index.html"
	} else if s.Index[0] != '/' {
		s.Index = "/" + s.Index
	}
	s.handler = handler(s.Root, s.SPA, s.Index)

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
		next := s.handler
		s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.middleware.ServeHTTP(next.ServeHTTP, w, r)
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

	if len(s.Rules) > 0 {
		s.handler = s.Rules.BuildHandler(s.handler.ServeHTTP)
	}

	if s.UseHealthCheck() {
		s.HealthMon = monitor.NewMonitor(s)
		if err := s.HealthMon.Start(s.task); err != nil {
			l := log.With().Str("type", "fileserver").Str("name", s.Name()).Logger()
			gperr.LogWarn("health monitor error", err, &l)
			s.HealthMon = nil
		}
	}

	ep := entrypoint.FromCtx(parent.Context())
	if ep == nil {
		return gperr.New("entrypoint not initialized")
	}
	ep.AddRoute(s)
	return nil
}

func (s *FileServer) RootPath() string {
	return s.Root
}

// ServeHTTP implements http.Handler.
func (s *FileServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if s.accessLogger != nil {
		rec := accesslog.GetResponseRecorder(w)
		w = rec
		defer func() {
			s.accessLogger.LogRequest(req, rec.Response())
			accesslog.PutResponseRecorder(rec)
		}()
	}
	s.handler.ServeHTTP(w, req)
}

package types

import (
	"net/http"

	idlewatcher "github.com/yusing/go-proxy/internal/docker/idlewatcher/types"
	net "github.com/yusing/go-proxy/internal/net/types"
	U "github.com/yusing/go-proxy/internal/utils"
	F "github.com/yusing/go-proxy/internal/utils/functional"
	"github.com/yusing/go-proxy/internal/watcher/health"
)

type (
	server struct {
		_ U.NoCopy

		name   string
		url    *net.URL
		weight Weight

		http.Handler `json:"-"`
		health.HealthMonitor
	}

	Server interface {
		http.Handler
		health.HealthMonitor
		Name() string
		Key() string
		URL() *net.URL
		Weight() Weight
		SetWeight(weight Weight)
		TryWake() error
	}

	Pool = F.Map[string, Server]
)

var NewServerPool = F.NewMap[Pool]

func NewServer(name string, url *net.URL, weight Weight, handler http.Handler, healthMon health.HealthMonitor) Server {
	srv := &server{
		name:          name,
		url:           url,
		weight:        weight,
		Handler:       handler,
		HealthMonitor: healthMon,
	}
	return srv
}

func TestNewServer[T ~int | ~float32 | ~float64](weight T) Server {
	srv := &server{
		weight: Weight(weight),
		url:    net.MustParseURL("http://localhost"),
	}
	return srv
}

func (srv *server) Name() string {
	return srv.name
}

func (srv *server) URL() *net.URL {
	return srv.url
}

func (srv *server) Key() string {
	return srv.url.Host
}

func (srv *server) Weight() Weight {
	return srv.weight
}

func (srv *server) SetWeight(weight Weight) {
	srv.weight = weight
}

func (srv *server) String() string {
	return srv.name
}

func (srv *server) TryWake() error {
	waker, ok := srv.Handler.(idlewatcher.Waker)
	if ok {
		return waker.Wake()
	}
	return nil
}

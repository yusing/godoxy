package loadbalancer

import (
	"context"
	"net/http"

	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/types"
)

type server struct {
	name   string
	url    *nettypes.URL
	weight int

	http.Handler `json:"-"`
	types.HealthMonitor
}

func NewServer(name string, url *nettypes.URL, weight int, handler http.Handler, healthMon types.HealthMonitor) types.LoadBalancerServer {
	srv := &server{
		name:          name,
		url:           url,
		weight:        weight,
		Handler:       handler,
		HealthMonitor: healthMon,
	}
	return srv
}

func TestNewServer[T ~int | ~float32 | ~float64](weight T) types.LoadBalancerServer {
	srv := &server{
		weight: int(weight),
		url:    nettypes.MustParseURL("http://localhost"),
	}
	return srv
}

func (srv *server) Name() string {
	return srv.name
}

func (srv *server) URL() *nettypes.URL {
	return srv.url
}

func (srv *server) Key() string {
	return srv.url.Host
}

func (srv *server) Weight() int {
	return srv.weight
}

func (srv *server) SetWeight(weight int) {
	srv.weight = weight
}

func (srv *server) String() string {
	return srv.name
}

func (srv *server) TryWake() error {
	waker, ok := srv.Handler.(idlewatcher.Waker)
	if ok {
		return waker.Wake(context.Background())
	}
	return nil
}

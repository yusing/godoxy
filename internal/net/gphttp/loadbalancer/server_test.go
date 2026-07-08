package loadbalancer

import (
	"net/url"

	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/types"
)

var testServerURL = nettypes.URL{URL: url.URL{Scheme: "http", Host: "localhost"}}

func newTestServer(weight float64) types.LoadBalancerServer {
	u := testServerURL
	return &server{
		weight: int(weight),
		url:    &u,
	}
}

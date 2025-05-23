package handler

import (
	"fmt"
	"net/http"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/agent/pkg/env"
	"github.com/yusing/go-proxy/internal/metrics/systeminfo"
	"github.com/yusing/go-proxy/pkg"
	socketproxy "github.com/yusing/go-proxy/socketproxy/pkg"
)

type ServeMux struct{ *http.ServeMux }

func (mux ServeMux) HandleEndpoint(method, endpoint string, handler http.HandlerFunc) {
	mux.ServeMux.HandleFunc(method+" "+agent.APIEndpointBase+endpoint, handler)
}

func (mux ServeMux) HandleFunc(endpoint string, handler http.HandlerFunc) {
	mux.ServeMux.HandleFunc(agent.APIEndpointBase+endpoint, handler)
}

func NewAgentHandler() http.Handler {
	mux := ServeMux{http.NewServeMux()}

	mux.HandleFunc(agent.EndpointProxyHTTP+"/{path...}", ProxyHTTP)
	mux.HandleEndpoint("GET", agent.EndpointVersion, pkg.GetVersionHTTPHandler())
	mux.HandleEndpoint("GET", agent.EndpointName, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, env.AgentName)
	})
	mux.HandleEndpoint("GET", agent.EndpointHealth, CheckHealth)
	mux.HandleEndpoint("GET", agent.EndpointSystemInfo, systeminfo.Poller.ServeHTTP)
	mux.ServeMux.HandleFunc("/", socketproxy.DockerSocketHandler(env.DockerSocket))
	return mux
}

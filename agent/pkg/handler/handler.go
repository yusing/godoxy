package handler

import (
	"fmt"
	"io"
	"net/http"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/agent/pkg/env"
	"github.com/yusing/go-proxy/internal/logging/memlogger"
	"github.com/yusing/go-proxy/internal/metrics/systeminfo"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"github.com/yusing/go-proxy/pkg"
)

type ServeMux struct{ *http.ServeMux }

func (mux ServeMux) HandleMethods(methods, endpoint string, handler http.HandlerFunc) {
	for _, m := range strutils.CommaSeperatedList(methods) {
		mux.ServeMux.HandleFunc(m+" "+agent.APIEndpointBase+endpoint, handler)
	}
}

func (mux ServeMux) HandleFunc(endpoint string, handler http.HandlerFunc) {
	mux.ServeMux.HandleFunc(agent.APIEndpointBase+endpoint, handler)
}

type NopWriteCloser struct {
	io.Writer
}

func (NopWriteCloser) Close() error {
	return nil
}

func NewAgentHandler() http.Handler {
	mux := ServeMux{http.NewServeMux()}

	mux.HandleFunc(agent.EndpointProxyHTTP+"/{path...}", ProxyHTTP)
	mux.HandleMethods("GET", agent.EndpointVersion, pkg.GetVersionHTTPHandler())
	mux.HandleMethods("GET", agent.EndpointName, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, env.AgentName)
	})
	mux.HandleMethods("GET", agent.EndpointHealth, CheckHealth)
	mux.HandleMethods("GET", agent.EndpointLogs, memlogger.HandlerFunc())
	mux.HandleMethods("GET", agent.EndpointSystemInfo, systeminfo.Poller.ServeHTTP)
	mux.ServeMux.HandleFunc("/", DockerSocketHandler())
	return mux
}

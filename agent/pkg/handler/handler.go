package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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

var upgrader = &websocket.Upgrader{
	// no origin check needed for internal websocket
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func NewAgentHandler() http.Handler {
	gin.SetMode(gin.ReleaseMode)
	mux := ServeMux{http.NewServeMux()}

	metricsHandler := gin.Default()
	{
		metrics := metricsHandler.Group(agent.APIEndpointBase)
		metrics.GET(agent.EndpointSystemInfo, func(c *gin.Context) {
			c.Set("upgrader", upgrader)
			systeminfo.Poller.ServeHTTP(c)
		})
	}

	mux.HandleFunc(agent.EndpointProxyHTTP+"/{path...}", ProxyHTTP)
	mux.HandleEndpoint("GET", agent.EndpointVersion, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, pkg.GetVersion())
	})
	mux.HandleEndpoint("GET", agent.EndpointName, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, env.AgentName)
	})
	mux.HandleEndpoint("GET", agent.EndpointHealth, CheckHealth)
	mux.HandleEndpoint("GET", agent.EndpointSystemInfo, metricsHandler.ServeHTTP)
	mux.ServeMux.HandleFunc("/", socketproxy.DockerSocketHandler(env.DockerSocket))
	return mux
}

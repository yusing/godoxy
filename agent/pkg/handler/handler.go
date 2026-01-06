package handler

import (
	"net/http"

	"github.com/bytedance/sonic"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/env"
	"github.com/yusing/godoxy/internal/metrics/systeminfo"
	socketproxy "github.com/yusing/godoxy/socketproxy/pkg"
	"github.com/yusing/goutils/version"
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
	mux.HandleFunc(agent.EndpointInfo, func(w http.ResponseWriter, r *http.Request) {
		agentInfo := agent.AgentInfo{
			Version:    version.Get(),
			Name:       env.AgentName,
			Runtime:    env.Runtime,
			StreamPort: env.AgentStreamPort,
		}
		sonic.ConfigDefault.NewEncoder(w).Encode(agentInfo)
	})
	mux.HandleEndpoint("GET", agent.EndpointHealth, CheckHealth)
	mux.HandleEndpoint("GET", agent.EndpointSystemInfo, metricsHandler.ServeHTTP)
	mux.ServeMux.HandleFunc("/", socketproxy.DockerSocketHandler(env.DockerSocket))
	return mux
}

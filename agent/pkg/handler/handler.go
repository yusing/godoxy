package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/env"
	"github.com/yusing/godoxy/internal/metrics/systeminfo"
	socketproxy "github.com/yusing/godoxy/socketproxy/pkg"
	strutils "github.com/yusing/goutils/strings"
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

	mux.HandleFunc(agent.EndpointInfo, func(w http.ResponseWriter, r *http.Request) {
		agentInfo := agent.AgentInfo{
			Version: version.Get(),
			Name:    env.AgentName,
			Runtime: env.Runtime,
		}
		w.Header().Set("Content-Type", "application/json")
		strutils.NewJSONEncoder(w).Encode(agentInfo)
	})
	mux.HandleEndpoint("GET", agent.EndpointHealth, CheckHealth)
	mux.HandleEndpoint("GET", agent.EndpointSystemInfo, metricsHandler.ServeHTTP)
	mux.ServeMux.HandleFunc("/", socketproxy.DockerSocketHandler(env.DockerSocket))

	// ServeMux canonicalizes paths containing repeated slashes before dispatch
	// and would expose the private proxy prefix in its redirect Location. Match
	// proxy requests first so ProxyHTTP can remove the prefix before the origin
	// decides whether the public path needs canonicalization.
	proxyPrefix := agent.APIEndpointBase + agent.EndpointProxyHTTP + "/"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, proxyPrefix) {
			ProxyHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

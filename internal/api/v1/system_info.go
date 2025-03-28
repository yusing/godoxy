package v1

import (
	"net/http"

	agentPkg "github.com/yusing/go-proxy/agent/pkg/agent"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/metrics/systeminfo"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
)

func SystemInfo(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	agentAddr := query.Get("agent_addr")
	query.Del("agent_addr")
	if agentAddr == "" {
		systeminfo.Poller.ServeHTTP(w, r)
		return
	}

	agent, ok := cfg.GetAgent(agentAddr)
	if !ok {
		gphttp.NotFound(w, "agent_addr")
		return
	}

	isWS := httpheaders.IsWebsocket(r.Header)
	if !isWS {
		respData, status, err := agent.Forward(r, agentPkg.EndpointSystemInfo)
		if err != nil {
			gphttp.ServerError(w, r, gperr.Wrap(err, "failed to forward request to agent"))
			return
		}
		if status != http.StatusOK {
			http.Error(w, string(respData), status)
			return
		}
		gphttp.WriteBody(w, respData)
	} else {
		rp := reverseproxy.NewReverseProxy("agent", agentPkg.AgentURL, agent.Transport())
		header := r.Header.Clone()
		r, err := http.NewRequestWithContext(r.Context(), r.Method, agentPkg.EndpointSystemInfo+"?"+query.Encode(), nil)
		if err != nil {
			gphttp.ServerError(w, r, gperr.Wrap(err, "failed to create request"))
			return
		}
		r.Header = header
		rp.ServeHTTP(w, r)
	}
}

package metrics

import (
	"net/http"

	"github.com/gin-gonic/gin"
	agentPkg "github.com/yusing/go-proxy/agent/pkg/agent"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	"github.com/yusing/go-proxy/internal/metrics/period"
	"github.com/yusing/go-proxy/internal/metrics/systeminfo"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
)

type SystemInfoRequest struct {
	AgentAddr string                             `query:"agent_addr"`
	Aggregate systeminfo.SystemInfoAggregateMode `query:"aggregate"`
	Period    period.Filter                      `query:"period"`
} // @name SystemInfoRequest

type SystemInfoAggregate period.ResponseType[systeminfo.Aggregated] // @name SystemInfoAggregate

// @x-id				"system_info"
// @BasePath		/api/v1
// @Summary		Get system info
// @Description	Get system info
// @Tags			metrics,websocket
// @Produce		json
// @Param			request	query		SystemInfoRequest	false	"Request"
// @Success		200			{object}	systeminfo.SystemInfo "no period specified"
// @Success		200			{object}	SystemInfoAggregate "period specified"
// @Failure		400			{object}	apitypes.ErrorResponse
// @Failure		403			{object}	apitypes.ErrorResponse
// @Failure		404			{object}	apitypes.ErrorResponse
// @Failure		500			{object}	apitypes.ErrorResponse
// @Router	 /metrics/system_info [get]
func SystemInfo(c *gin.Context) {
	query := c.Request.URL.Query()
	agentAddr := query.Get("agent_addr")
	query.Del("agent_addr")
	if agentAddr == "" {
		systeminfo.Poller.ServeHTTP(c)
		return
	}

	agent, ok := agentPkg.GetAgent(agentAddr)
	if !ok {
		c.JSON(http.StatusNotFound, apitypes.Error("agent_addr not found"))
		return
	}

	isWS := httpheaders.IsWebsocket(c.Request.Header)
	if !isWS {
		respData, status, err := agent.Forward(c.Request, agentPkg.EndpointSystemInfo)
		if err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to forward request to agent"))
			return
		}
		if status != http.StatusOK {
			c.JSON(status, apitypes.Error(string(respData)))
			return
		}
		c.JSON(status, respData)
	} else {
		rp := reverseproxy.NewReverseProxy("agent", nettypes.NewURL(agentPkg.AgentURL), agent.Transport())
		header := c.Request.Header.Clone()
		r, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, agentPkg.EndpointSystemInfo+"?"+query.Encode(), nil)
		if err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to create request"))
			return
		}
		r.Header = header
		rp.ServeHTTP(c.Writer, r)
	}
}

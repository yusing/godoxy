package metrics

import (
	"io"
	"maps"
	"net/http"

	"github.com/gin-gonic/gin"
	agentPkg "github.com/yusing/godoxy/agent/pkg/agent"
	apitypes "github.com/yusing/godoxy/internal/api/types"
	"github.com/yusing/godoxy/internal/metrics/period"
	"github.com/yusing/godoxy/internal/metrics/systeminfo"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/synk"
)

type SystemInfoRequest struct {
	AgentAddr string                             `query:"agent_addr"`
	AgentName string                             `query:"agent_name"`
	Aggregate systeminfo.SystemInfoAggregateMode `query:"aggregate"`
	Period    period.Filter                      `query:"period"`
} // @name SystemInfoRequest

type SystemInfoAggregate period.ResponseType[systeminfo.AggregatedJSON] // @name SystemInfoAggregate

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
	agentName := query.Get("agent_name")
	query.Del("agent_addr")
	query.Del("agent_name")
	if agentAddr == "" && agentName == "" {
		systeminfo.Poller.ServeHTTP(c)
		return
	}
	c.Request.URL.RawQuery = query.Encode()

	agent, ok := agentPkg.GetAgent(agentAddr)
	if !ok {
		agent, ok = agentPkg.GetAgentByName(agentName)
	}
	if !ok {
		c.JSON(http.StatusNotFound, apitypes.Error("agent_addr or agent_name not found"))
		return
	}

	isWS := httpheaders.IsWebsocket(c.Request.Header)
	if !isWS {
		resp, err := agent.Forward(c.Request, agentPkg.EndpointSystemInfo)
		if err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to forward request to agent"))
			return
		}
		defer resp.Body.Close()

		maps.Copy(c.Writer.Header(), resp.Header)
		c.Status(resp.StatusCode)

		buf := pool.Get()
		defer pool.Put(buf)
		io.CopyBuffer(c.Writer, resp.Body, buf)
	} else {
		agent.ReverseProxy(c.Writer, c.Request, agentPkg.EndpointSystemInfo)
	}
}

var pool = synk.GetBytesPool()

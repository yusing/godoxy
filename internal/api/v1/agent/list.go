package agentapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"

	_ "github.com/yusing/goutils/apitypes"
)

// @x-id				"list"
// @BasePath		/api/v1
// @Summary		List agents
// @Description	List agents
// @Tags			agent,websocket
// @Accept			json
// @Produce		json
// @Success		200	{array}		agent.AgentConfig
// @Failure		403	{object}	apitypes.ErrorResponse
// @Router			/agent/list [get]
func List(c *gin.Context) {
	if httpheaders.IsWebsocket(c.Request.Header) {
		websocket.PeriodicWrite(c, 10*time.Second, func() (any, error) {
			return agentpool.List(), nil
		})
	} else {
		c.JSON(http.StatusOK, agentpool.List())
	}
}

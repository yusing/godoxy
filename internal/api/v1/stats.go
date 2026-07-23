package v1

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	statequery "github.com/yusing/godoxy/internal/config/query"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"
)

type StatsResponse struct {
	Proxies ProxyStats                  `json:"proxies"`
	Uptime  int64                       `json:"uptime"`
	Runtime configtypes.RuntimeSnapshot `json:"runtime"`
} //	@name	StatsResponse

type RouteStats = routing.RouteStats // @name RouteStats

type ProviderStats struct {
	Total          uint16               `json:"total"`
	ReverseProxies RouteStats           `json:"reverse_proxies"`
	Streams        RouteStats           `json:"streams"`
	Type           routing.ProviderType `json:"type"`
} //	@name	ProviderStats

type ProxyStats struct {
	Total          uint16                   `json:"total"`
	ReverseProxies RouteStats               `json:"reverse_proxies"`
	Streams        RouteStats               `json:"streams"`
	Providers      map[string]ProviderStats `json:"providers"`
} //	@name	ProxyStats

// @x-id				"stats"
// @BasePath		/api/v1
// @Summary		Get GoDoxy stats
// @Description	Get stats
// @Tags			v1,websocket
// @Accept			json
// @Produce		json
// @Success		200	{object}	StatsResponse
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/stats [get]
func Stats(c *gin.Context) {
	ctx := c.Request.Context()
	getStats := func() (any, error) {
		state := configtypes.FromCtx(ctx)
		return map[string]any{
			"proxies": statequery.GetStatistics(ctx),
			"uptime":  int64(time.Since(startTime).Round(time.Second).Seconds()),
			"runtime": state.RuntimeSnapshot(),
		}, nil
	}

	if httpheaders.IsWebsocket(c.Request.Header) {
		websocket.PeriodicWrite(c, time.Second, getStats)
	} else {
		stats, _ := getStats()
		c.JSON(http.StatusOK, stats)
	}
}

var startTime = time.Now()

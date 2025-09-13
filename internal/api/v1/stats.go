package v1

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/net/gphttp/websocket"
	"github.com/yusing/go-proxy/internal/types"
)

type StatsResponse struct {
	Proxies ProxyStats `json:"proxies"`
	Uptime  int64      `json:"uptime"`
} //	@name	StatsResponse

type ProxyStats struct {
	Total          uint16                         `json:"total"`
	ReverseProxies types.RouteStats               `json:"reverse_proxies"`
	Streams        types.RouteStats               `json:"streams"`
	Providers      map[string]types.ProviderStats `json:"providers"`
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
	cfg := config.GetInstance()
	getStats := func() (any, error) {
		return map[string]any{
			"proxies": cfg.Statistics(),
			"uptime":  int64(time.Since(startTime).Round(time.Second).Seconds()),
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

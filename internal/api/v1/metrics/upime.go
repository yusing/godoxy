package metrics

import (
	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/metrics/period"
	"github.com/yusing/godoxy/internal/metrics/uptime"
)

type UptimeRequest struct {
	Limit    int           `query:"limit" example:"10" default:"0"`
	Offset   int           `query:"offset" example:"10" default:"0"`
	Interval period.Filter `query:"interval" example:"1m"`
	Keyword  string        `query:"keyword" example:""`
} // @name UptimeRequest

type UptimeAggregate period.ResponseType[uptime.Aggregated] // @name UptimeAggregate

// @x-id				"uptime"
// @BasePath		/api/v1
// @Summary		Get uptime
// @Description	Get uptime
// @Tags			metrics,websocket
// @Produce   json
// @Param			request	query		UptimeRequest	false	"Request"
// @Success		200		{object}	uptime.StatusByAlias "no period specified"
// @Success		200		{object}	UptimeAggregate "period specified"
// @Success   204   {object}	apitypes.ErrorResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		403		{object}	apitypes.ErrorResponse
// @Failure		500		{object}	apitypes.ErrorResponse
// @Router			/metrics/uptime [get]
func Uptime(c *gin.Context) {
	uptime.Poller.ServeHTTP(c)
}

package period

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	metricsutils "github.com/yusing/go-proxy/internal/metrics/utils"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/net/gphttp/websocket"
)

type ResponseType[AggregateT any] struct {
	Total int        `json:"total"`
	Data  AggregateT `json:"data"`
}

// ServeHTTP serves the data for the given period.
//
// If the period is not specified, it serves the last result.
//
// If the period is specified, it serves the data for the given period.
//
// If the period is invalid, it returns a 400 error.
//
// If the data is not found, it returns a 204 error.
//
// If the request is a websocket request, it serves the data for the given period for every interval.
func (p *Poller[T, AggregateT]) ServeHTTP(c *gin.Context) {
	period := Filter(c.Query("period"))
	query := c.Request.URL.Query()

	if httpheaders.IsWebsocket(c.Request.Header) {
		interval := metricsutils.QueryDuration(query, "interval", 0)

		minInterval := 1 * time.Second
		if interval == 0 {
			interval = pollInterval
		}
		if interval < minInterval {
			interval = minInterval
		}
		websocket.PeriodicWrite(c, interval, func() (any, error) {
			return p.getRespData(period, query)
		})
	} else {
		data, err := p.getRespData(period, query)
		if err != nil {
			c.Error(apitypes.InternalServerError(err, "failed to get response data"))
			return
		}
		if data == nil {
			c.JSON(http.StatusNoContent, apitypes.Error("no data"))
			return
		}
		c.JSON(http.StatusOK, data)
	}
}

func (p *Poller[T, AggregateT]) getRespData(period Filter, query url.Values) (any, error) {
	if period == "" {
		return p.GetLastResult(), nil
	}
	rangeData, ok := p.Get(period)
	if !ok {
		return nil, errors.New("invalid period")
	}
	total, aggregated := p.aggregate(rangeData, query)
	if total == -1 {
		return nil, errors.New("bad request")
	}
	return map[string]any{
		"total": total,
		"data":  aggregated,
	}, nil
}

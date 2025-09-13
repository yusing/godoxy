package period

import (
	"errors"
	"net/http"
	"net/url"

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

		if interval < PollInterval {
			interval = PollInterval
		}
		websocket.PeriodicWrite(c, interval, func() (any, error) {
			return p.GetRespData(period, query)
		})
	} else {
		data, err := p.GetRespData(period, query)
		if err != nil {
			c.JSON(http.StatusBadRequest, apitypes.Error("bad request", err))
			return
		}
		if data == nil {
			c.JSON(http.StatusNoContent, apitypes.Error("no data"))
			return
		}
		c.JSON(http.StatusOK, data)
	}
}

// GetRespData returns the aggregated data for the given period and query.
//
// When period is specified:
//
//	It returns a map with the total and the data.
//	It returns an error if the period or query is invalid.
//
// When period is not specified:
//
//	It returns the last result.
//	It returns nil if no last result is found.
func (p *Poller[T, AggregateT]) GetRespData(period Filter, query url.Values) (any, error) {
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

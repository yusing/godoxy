package period

import (
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	metricsutils "github.com/yusing/go-proxy/internal/metrics/utils"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
)

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
func (p *Poller[T, AggregateT]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	if httpheaders.IsWebsocket(r.Header) {
		interval := metricsutils.QueryDuration(query, "interval", 0)

		minInterval := 1 * time.Second
		if interval == 0 {
			interval = pollInterval
		}
		if interval < minInterval {
			interval = minInterval
		}
		gpwebsocket.Periodic(w, r, interval, func(conn *websocket.Conn) error {
			data, err := p.getRespData(r)
			if err != nil {
				return err
			}
			if data == nil {
				return nil
			}
			return wsjson.Write(r.Context(), conn, data)
		})
	} else {
		data, err := p.getRespData(r)
		if err != nil {
			gphttp.ServerError(w, r, err)
			return
		}
		if data == nil {
			http.Error(w, "no data", http.StatusNoContent)
			return
		}
		gphttp.RespondJSON(w, r, data)
	}
}

func (p *Poller[T, AggregateT]) getRespData(r *http.Request) (any, error) {
	query := r.URL.Query()
	period := query.Get("period")
	if period == "" {
		return p.GetLastResult(), nil
	}
	rangeData, ok := p.Get(Filter(period))
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

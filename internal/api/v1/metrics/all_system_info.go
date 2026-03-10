package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/metrics/period"
	"github.com/yusing/godoxy/internal/metrics/systeminfo"
	apitypes "github.com/yusing/goutils/apitypes"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"
	"github.com/yusing/goutils/synk"
)

var bytesPool = synk.GetUnsizedBytesPool()

type AllSystemInfoRequest struct {
	Period    period.Filter                      `query:"period"`
	Aggregate systeminfo.SystemInfoAggregateMode `query:"aggregate"`
	Interval  time.Duration                      `query:"interval" swaggertype:"string" format:"duration"`
} // @name AllSystemInfoRequest

type bytesFromPool struct {
	json.RawMessage
	release func([]byte)
}

type systemInfoData struct {
	agentName  string
	systemInfo any
}

// @x-id				"all_system_info"
// @BasePath		/api/v1
// @Summary		Get system info
// @Description	Get system info
// @Tags			metrics,websocket
// @Produce		json
// @Param			request	query		AllSystemInfoRequest	false	"Request"
// @Success		200			{object}	map[string]systeminfo.SystemInfo "no period specified, system info by agent name"
// @Success		200			{object}	map[string]SystemInfoAggregate "period specified, aggregated system info by agent name"
// @Failure		400			{object}	apitypes.ErrorResponse
// @Failure		403			{object}	apitypes.ErrorResponse
// @Failure		500			{object}	apitypes.ErrorResponse
// @Router	 /metrics/all_system_info [get]
func AllSystemInfo(c *gin.Context) {
	var req AllSystemInfoRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid query", err))
		return
	}

	if req.Interval < period.PollInterval {
		req.Interval = period.PollInterval
	}

	if !httpheaders.IsWebsocket(c.Request.Header) {
		c.JSON(http.StatusBadRequest, apitypes.Error("bad request, websocket is required"))
		return
	}

	manager, err := websocket.NewManagerWithUpgrade(c)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to upgrade to websocket"))
		return
	}
	defer manager.Close()

	query := c.Request.URL.Query()
	queryEncoded := query.Encode()

	// leave 5 extra slots for buffering in case new agents are added.
	dataCh := make(chan systemInfoData, 1+agentpool.Num()+5)

	ticker := time.NewTicker(req.Interval)
	defer ticker.Stop()

	go streamSystemInfo(manager, dataCh)

	// write system info immediately once.
	if hasSuccess, err := collectSystemInfoRound(manager, req, query, queryEncoded, dataCh); handleRoundResult(c, hasSuccess, err, false) {
		return
	}

	// then continue on the ticker.
	for {
		select {
		case <-manager.Done():
			return
		case <-ticker.C:
			if hasSuccess, err := collectSystemInfoRound(manager, req, query, queryEncoded, dataCh); handleRoundResult(c, hasSuccess, err, true) {
				return
			}
		}
	}
}

func streamSystemInfo(manager *websocket.Manager, dataCh <-chan systemInfoData) {
	for {
		select {
		case <-manager.Done():
			return
		case data := <-dataCh:
			err := marshalSystemInfo(manager, data.agentName, data.systemInfo)
			if err != nil {
				manager.Close()
				return
			}
		}
	}
}

func queueSystemInfo(manager *websocket.Manager, dataCh chan<- systemInfoData, data systemInfoData) {
	select {
	case <-manager.Done():
	case dataCh <- data:
	}
}

func collectSystemInfoRound(
	manager *websocket.Manager,
	req AllSystemInfoRequest,
	query url.Values,
	queryEncoded string,
	dataCh chan<- systemInfoData,
) (hasSuccess bool, err error) {
	var numErrs atomic.Int32
	totalAgents := int32(1) // myself

	var errs gperr.Group
	// get system info for me and all agents in parallel.
	errs.Go(func() error {
		data, err := systeminfo.Poller.GetRespData(req.Period, query)
		if err != nil {
			numErrs.Add(1)
			return gperr.PrependSubject(err, "Main server")
		}
		queueSystemInfo(manager, dataCh, systemInfoData{
			agentName:  "GoDoxy",
			systemInfo: data,
		})
		return nil
	})

	for _, a := range agentpool.Iter() {
		totalAgents++

		errs.Go(func() error {
			data, err := getAgentSystemInfoWithRetry(manager.Context(), a, queryEncoded)
			if err != nil {
				numErrs.Add(1)
				return gperr.PrependSubject(err, "Agent "+a.Name)
			}
			queueSystemInfo(manager, dataCh, systemInfoData{
				agentName:  a.Name,
				systemInfo: data,
			})
			return nil
		})
	}

	err = errs.Wait().Error()
	return numErrs.Load() < totalAgents, err
}

func handleRoundResult(c *gin.Context, hasSuccess bool, err error, logPartial bool) (stop bool) {
	if err == nil {
		return false
	}
	if !hasSuccess {
		c.Error(apitypes.InternalServerError(err, "failed to get all system info"))
		return true
	}
	if logPartial {
		log.Warn().Err(err).Msg("failed to get some system info")
	}
	return false
}

func getAgentSystemInfo(ctx context.Context, a *agentpool.Agent, query string) (bytesFromPool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	path := agent.EndpointSystemInfo + "?" + query
	resp, err := a.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return bytesFromPool{}, err
	}
	defer resp.Body.Close()

	// NOTE: buffer will be released by marshalSystemInfo once marshaling is done.
	bytesBuf, release, err := httputils.ReadAllBody(resp)
	if err != nil {
		return bytesFromPool{}, err
	}
	return bytesFromPool{json.RawMessage(bytesBuf), release}, nil
}

func getAgentSystemInfoWithRetry(ctx context.Context, a *agentpool.Agent, query string) (bytesFromPool, error) {
	const maxRetries = 3
	const retryDelay = 5 * time.Second
	var attempt int
	data, err := backoff.Retry(ctx, func() (bytesFromPool, error) {
		attempt++

		data, err := getAgentSystemInfo(ctx, a, query)
		if err == nil {
			return data, nil
		}

		log.Err(err).Str("agent", a.Name).Int("attempt", attempt).Msg("Agent request attempt failed")
		return bytesFromPool{}, err
	},
		backoff.WithBackOff(backoff.NewConstantBackOff(retryDelay)),
		backoff.WithMaxTries(maxRetries),
	)
	if err != nil {
		return bytesFromPool{}, err
	}
	return data, nil
}

func marshalSystemInfo(ws *websocket.Manager, agentName string, systemInfo any) error {
	buf := bytesPool.GetBuffer()
	defer bytesPool.PutBuffer(buf)

	// release the buffer retrieved from getAgentSystemInfo
	if bufFromPool, ok := systemInfo.(bytesFromPool); ok {
		defer bufFromPool.release(bufFromPool.RawMessage)
	}

	err := json.NewEncoder(buf).Encode(map[string]any{
		agentName: systemInfo,
	})
	if err != nil {
		return err
	}

	return ws.WriteData(websocket.TextMessage, buf.Bytes(), 3*time.Second)
}

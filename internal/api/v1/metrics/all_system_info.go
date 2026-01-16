package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

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
	queryEncoded := c.Request.URL.Query().Encode()

	type SystemInfoData struct {
		AgentName  string
		SystemInfo any
	}

	// leave 5 extra slots for buffering in case new agents are added.
	dataCh := make(chan SystemInfoData, 1+agentpool.Num()+5)
	defer close(dataCh)

	ticker := time.NewTicker(req.Interval)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-manager.Done():
				return
			case data := <-dataCh:
				err := marshalSystemInfo(manager, data.AgentName, data.SystemInfo)
				if err != nil {
					manager.Close()
					return
				}
			}
		}
	}()

	// processing function for one round.
	doRound := func() (bool, error) {
		var numErrs atomic.Int32

		totalAgents := int32(1) // myself

		var errs gperr.Group
		// get system info for me and all agents in parallel.
		errs.Go(func() error {
			data, err := systeminfo.Poller.GetRespData(req.Period, query)
			if err != nil {
				numErrs.Add(1)
				return gperr.PrependSubject("Main server", err)
			}
			select {
			case <-manager.Done():
				return nil
			case dataCh <- SystemInfoData{
				AgentName:  "GoDoxy",
				SystemInfo: data,
			}:
			}
			return nil
		})

		for _, a := range agentpool.Iter() {
			totalAgents++

			errs.Go(func() error {
				data, err := getAgentSystemInfoWithRetry(manager.Context(), a, queryEncoded)
				if err != nil {
					numErrs.Add(1)
					return gperr.PrependSubject("Agent "+a.Name, err)
				}
				select {
				case <-manager.Done():
					return nil
				case dataCh <- SystemInfoData{
					AgentName:  a.Name,
					SystemInfo: data,
				}:
				}
				return nil
			})
		}

		err := errs.Wait().Error()
		return numErrs.Load() == totalAgents, err
	}

	// write system info immediately once.
	if shouldContinue, err := doRound(); err != nil {
		if !shouldContinue {
			c.Error(apitypes.InternalServerError(err, "failed to get all system info"))
			return
		}
	}

	// then continue on the ticker.
	for {
		select {
		case <-manager.Done():
			return
		case <-ticker.C:
			if shouldContinue, err := doRound(); err != nil {
				if !shouldContinue {
					c.Error(apitypes.InternalServerError(err, "failed to get all system info"))
					return
				}
				gperr.LogWarn("failed to get some system info", err)
			}
		}
	}
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
	var lastErr error

	for attempt := range maxRetries {
		// Apply backoff delay for retries (not for first attempt)
		if attempt > 0 {
			delay := max((1<<attempt)*time.Second, 5*time.Second)
			select {
			case <-ctx.Done():
				return bytesFromPool{}, ctx.Err()
			case <-time.After(delay):
			}
		}

		data, err := getAgentSystemInfo(ctx, a, query)
		if err == nil {
			return data, nil
		}

		lastErr = err

		log.Debug().Str("agent", a.Name).Int("attempt", attempt+1).Str("error", err.Error()).Msg("Agent request attempt failed")

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return bytesFromPool{}, ctx.Err()
		}
	}

	return bytesFromPool{}, lastErr
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

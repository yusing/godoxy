package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/agent/pkg/agent"
	apitypes "github.com/yusing/godoxy/internal/api/types"
	"github.com/yusing/godoxy/internal/metrics/period"
	"github.com/yusing/godoxy/internal/metrics/systeminfo"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/websocket"
	"github.com/yusing/goutils/synk"
)

var (
	// for json marshaling (unknown size)
	allSystemInfoBytesPool = synk.GetBytesPoolWithUniqueMemory()
	// for storing http response body (known size)
	allSystemInfoFixedSizePool = synk.GetBytesPool()
)

type AllSystemInfoRequest struct {
	Period    period.Filter                      `query:"period"`
	Aggregate systeminfo.SystemInfoAggregateMode `query:"aggregate"`
	Interval  time.Duration                      `query:"interval" swaggertype:"string" format:"duration"`
} // @name AllSystemInfoRequest

type bytesFromPool struct {
	json.RawMessage
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
	dataCh := make(chan SystemInfoData, 1+agent.NumAgents()+5)
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
		var roundWg sync.WaitGroup
		var numErrs atomic.Int32

		totalAgents := int32(1) // myself

		errs := gperr.NewBuilderWithConcurrency()
		// get system info for me and all agents in parallel.
		roundWg.Go(func() {
			data, err := systeminfo.Poller.GetRespData(req.Period, query)
			if err != nil {
				errs.Add(gperr.Wrap(err, "Main server"))
				numErrs.Add(1)
				return
			}
			select {
			case <-manager.Done():
				return
			case dataCh <- SystemInfoData{
				AgentName:  "GoDoxy",
				SystemInfo: data,
			}:
			}
		})

		for _, a := range agent.IterAgents() {
			totalAgents++
			agentShallowCopy := *a

			roundWg.Go(func() {
				data, err := getAgentSystemInfoWithRetry(manager.Context(), &agentShallowCopy, queryEncoded)
				if err != nil {
					errs.Add(gperr.Wrap(err, "Agent "+agentShallowCopy.Name))
					numErrs.Add(1)
					return
				}
				select {
				case <-manager.Done():
					return
				case dataCh <- SystemInfoData{
					AgentName:  agentShallowCopy.Name,
					SystemInfo: data,
				}:
				}
			})
		}

		roundWg.Wait()
		return numErrs.Load() == totalAgents, errs.Error()
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

func getAgentSystemInfo(ctx context.Context, a *agent.AgentConfig, query string) (json.Marshaler, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	path := agent.EndpointSystemInfo + "?" + query
	resp, err := a.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// NOTE: buffer will be released by marshalSystemInfo once marshaling is done.
	if resp.ContentLength >= 0 {
		bytesBuf := allSystemInfoFixedSizePool.GetSized(int(resp.ContentLength))
		_, err = io.ReadFull(resp.Body, bytesBuf)
		if err != nil {
			// prevent pool leak on error.
			allSystemInfoFixedSizePool.Put(bytesBuf)
			return nil, err
		}
		return bytesFromPool{json.RawMessage(bytesBuf)}, nil
	}

	// Fallback when content length is unknown (should not happen but just in case).
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func getAgentSystemInfoWithRetry(ctx context.Context, a *agent.AgentConfig, query string) (json.Marshaler, error) {
	const maxRetries = 3
	var lastErr error

	for attempt := range maxRetries {
		// Apply backoff delay for retries (not for first attempt)
		if attempt > 0 {
			delay := max((1<<attempt)*time.Second, 5*time.Second)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
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
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
}

func marshalSystemInfo(ws *websocket.Manager, agentName string, systemInfo any) error {
	bytesBuf := allSystemInfoBytesPool.Get()
	defer allSystemInfoBytesPool.Put(bytesBuf)

	// release the buffer retrieved from getAgentSystemInfo
	if bufFromPool, ok := systemInfo.(bytesFromPool); ok {
		defer allSystemInfoFixedSizePool.Put(bufFromPool.RawMessage)
	}

	buf := bytes.NewBuffer(bytesBuf)
	err := json.NewEncoder(buf).Encode(map[string]any{
		agentName: systemInfo,
	})
	if err != nil {
		return err
	}

	return ws.WriteData(websocket.TextMessage, buf.Bytes(), 3*time.Second)
}

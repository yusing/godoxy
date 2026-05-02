package agentpool

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/valyala/fasthttp"
	agentPkg "github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/goutils/http/reverseproxy"
	strutils "github.com/yusing/goutils/strings"
)

func (agent *Agent) Do(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, agentPkg.APIBaseURL+endpoint, body)
	if err != nil {
		return nil, err
	}
	return agent.httpClient.Do(req)
}

func (agent *Agent) Forward(req *http.Request, endpoint string) (*http.Response, error) {
	req.URL.Host = agentPkg.AgentHost
	req.URL.Scheme = "https"
	req.URL.Path = agentPkg.APIEndpointBase + endpoint
	req.RequestURI = ""
	resp, err := agent.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

type HealthCheckResponse struct {
	Healthy bool          `json:"healthy"`
	Detail  string        `json:"detail"`
	Latency time.Duration `json:"latency"`
}

func (agent *Agent) DoHealthCheck(timeout time.Duration, query string) (ret HealthCheckResponse, err error) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(agentPkg.APIBaseURL + agentPkg.EndpointHealth + "?" + query)
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("Accept-Encoding", "identity")
	req.SetConnectionClose()

	start := time.Now()
	err = agent.fasthttpHcClient.DoTimeout(req, resp, timeout)
	ret.Latency = time.Since(start)
	if err != nil {
		return ret, err
	}

	if status := resp.StatusCode(); status != http.StatusOK {
		ret.Detail = fmt.Sprintf("HTTP %d %s", status, resp.Body())
		return ret, nil
	} else {
		err = strutils.UnmarshalJSON(resp.Body(), &ret)
		if err != nil {
			return ret, err
		}
	}
	return ret, nil
}

func (agent *Agent) Websocket(ctx context.Context, endpoint string) (*websocket.Conn, *http.Response, error) {
	transport := agent.Transport()
	dialer := websocket.Dialer{
		NetDialContext:    transport.DialContext,
		NetDialTLSContext: transport.DialTLSContext,
	}
	return dialer.DialContext(ctx, agentPkg.APIBaseURL+endpoint, http.Header{
		"Host": {agentPkg.AgentHost},
	})
}

// ReverseProxy reverse proxies the request to the agent
//
// It will create a new request with the same context, method, and body, but with the agent host and scheme, and the endpoint
// If the request has a query, it will be added to the proxy request's URL
func (agent *Agent) ReverseProxy(w http.ResponseWriter, req *http.Request, endpoint string) {
	rp := reverseproxy.NewReverseProxy("agent", agentPkg.AgentURL, agent.Transport())
	req.URL.Host = agentPkg.AgentHost
	req.URL.Scheme = "https"
	req.URL.Path = endpoint
	req.RequestURI = ""
	rp.ServeHTTP(w, req)
}

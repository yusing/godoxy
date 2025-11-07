package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gorilla/websocket"
	"github.com/valyala/fasthttp"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/http/reverseproxy"
)

func (cfg *AgentConfig) Do(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, APIBaseURL+endpoint, body)
	if err != nil {
		return nil, err
	}
	return cfg.httpClient.Do(req)
}

func (cfg *AgentConfig) Forward(req *http.Request, endpoint string) (*http.Response, error) {
	req.URL.Host = AgentHost
	req.URL.Scheme = "https"
	req.URL.Path = APIEndpointBase + endpoint
	req.RequestURI = ""
	resp, err := cfg.httpClient.Do(req)
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

func (cfg *AgentConfig) DoHealthCheck(timeout time.Duration, query string) (ret HealthCheckResponse, err error) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(APIBaseURL + EndpointHealth + "?" + query)
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("Accept-Encoding", "identity")
	req.SetConnectionClose()

	start := time.Now()
	err = cfg.fasthttpClientHealthCheck.DoTimeout(req, resp, timeout)
	ret.Latency = time.Since(start)
	if err != nil {
		return ret, err
	}

	if status := resp.StatusCode(); status != http.StatusOK {
		ret.Detail = fmt.Sprintf("HTTP %d %s", status, resp.Body())
		return ret, nil
	} else {
		err = sonic.Unmarshal(resp.Body(), &ret)
		if err != nil {
			return ret, err
		}
	}
	return ret, nil
}

func (cfg *AgentConfig) fetchString(ctx context.Context, endpoint string) (string, int, error) {
	resp, err := cfg.Do(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	data, release, err := httputils.ReadAllBody(resp)
	if err != nil {
		return "", 0, err
	}
	ret := string(data)
	release(data)
	return ret, resp.StatusCode, nil
}

func (cfg *AgentConfig) Websocket(ctx context.Context, endpoint string) (*websocket.Conn, *http.Response, error) {
	transport := cfg.Transport()
	dialer := websocket.Dialer{
		NetDialContext:    transport.DialContext,
		NetDialTLSContext: transport.DialTLSContext,
	}
	return dialer.DialContext(ctx, APIBaseURL+endpoint, http.Header{
		"Host": {AgentHost},
	})
}

// ReverseProxy reverse proxies the request to the agent
//
// It will create a new request with the same context, method, and body, but with the agent host and scheme, and the endpoint
// If the request has a query, it will be added to the proxy request's URL
func (cfg *AgentConfig) ReverseProxy(w http.ResponseWriter, req *http.Request, endpoint string) {
	rp := reverseproxy.NewReverseProxy("agent", AgentURL, cfg.Transport())
	req.URL.Host = AgentHost
	req.URL.Scheme = "https"
	req.URL.Path = endpoint
	req.RequestURI = ""
	rp.ServeHTTP(w, req)
}

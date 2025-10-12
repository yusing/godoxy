package agent

import (
	"context"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
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

func (cfg *AgentConfig) DoHealthCheck(ctx context.Context, endpoint string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", APIBaseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Connection", "close")

	return cfg.httpClientHealthCheck.Do(req)
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

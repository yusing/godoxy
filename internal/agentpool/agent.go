package agentpool

import (
	"net"
	"net/http"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/yusing/godoxy/agent/pkg/agent"
)

type Agent struct {
	*agent.AgentConfig

	httpClient       *http.Client
	fasthttpHcClient *fasthttp.Client
}

func newAgent(cfg *agent.AgentConfig) *Agent {
	transport := cfg.Transport()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 100
	transport.ReadBufferSize = 16384
	transport.WriteBufferSize = 16384

	return &Agent{
		AgentConfig: cfg,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Second,
		},
		fasthttpHcClient: &fasthttp.Client{
			DialTimeout: func(addr string, timeout time.Duration) (net.Conn, error) {
				if addr != agent.AgentHost+":443" {
					return nil, &net.AddrError{Err: "invalid address", Addr: addr}
				}
				return net.DialTimeout("tcp", cfg.Addr, timeout)
			},
			TLSConfig:                     cfg.TLSConfig(),
			ReadTimeout:                   5 * time.Second,
			WriteTimeout:                  3 * time.Second,
			DisableHeaderNamesNormalizing: true,
			DisablePathNormalizing:        true,
			NoDefaultUserAgentHeader:      true,
			ReadBufferSize:                1024,
			WriteBufferSize:               1024,
		},
	}
}

func (agent *Agent) HTTPClient() *http.Client {
	return &http.Client{
		Transport: agent.Transport(),
	}
}

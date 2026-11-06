package monitor_test

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/handler"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/health/monitor"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func TestAgentProxiedMonitorHTTPOptions(t *testing.T) {
	ca, serverPEM, clientPEM, err := agent.NewAgent()
	require.NoError(t, err)
	cert, err := serverPEM.ToTLSCert()
	require.NoError(t, err)
	agentServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case agent.APIEndpointBase + agent.EndpointHealth:
			handler.CheckHealth(w, r)
		case agent.APIEndpointBase + agent.EndpointName:
			io.WriteString(w, "health-test")
		case agent.APIEndpointBase + agent.EndpointVersion:
			io.WriteString(w, "v0.0.1")
		default:
			http.NotFound(w, r)
		}
	}))
	agentServer.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}}
	agentServer.StartTLS()
	t.Cleanup(agentServer.Close)
	cfg := &agent.AgentConfig{Addr: agentServer.Listener.Addr().String()}
	require.NoError(t, cfg.InitWithCerts(t.Context(), ca.Cert, clientPEM.Cert, clientPEM.Key))
	pool := agentpool.NewPool()
	require.True(t, pool.Add(cfg))
	remoteAgent, ok := pool.Get(cfg.Addr)
	require.True(t, ok)

	for _, scheme := range []string{"http", "https", "h2c"} {
		t.Run(scheme, func(t *testing.T) {
			for _, tt := range []struct {
				name, basePath, path, wantPath string
				useGet                         bool
			}{
				{name: "default HEAD", basePath: "/base", wantPath: "/base"},
				{name: "custom HEAD", basePath: "/base", path: "/ready", wantPath: "/base/ready"},
				{name: "default GET", useGet: true, basePath: "/base", wantPath: "/base"},
				{name: "custom GET", useGet: true, basePath: "/base", path: "/ready", wantPath: "/base/ready"},
				{name: "escaped path", useGet: true, basePath: "/base%2Fsegment", path: "/ready", wantPath: "/base%2Fsegment/ready"},
			} {
				t.Run(tt.name, func(t *testing.T) {
					method := http.MethodHead
					if tt.useGet {
						method = http.MethodGet
					}
					backendHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != method || r.URL.EscapedPath() != tt.wantPath {
							w.WriteHeader(http.StatusServiceUnavailable)
							return
						}
						if scheme == "h2c" && r.ProtoMajor != 2 {
							w.WriteHeader(http.StatusHTTPVersionNotSupported)
						}
					})
					backend := httptest.NewUnstartedServer(h2c.NewHandler(backendHandler, &http2.Server{}))
					if scheme == "https" {
						backend.StartTLS()
					} else {
						backend.Start()
					}
					t.Cleanup(backend.Close)
					target, err := url.Parse(backend.URL + tt.basePath)
					require.NoError(t, err)
					target.Scheme = scheme
					mon := monitor.NewAgentProxiedMonitor(health.HealthCheckConfig{
						UseGet: tt.useGet, Path: tt.path, Timeout: time.Second,
					}, remoteAgent, target)
					result, err := mon.CheckHealth()
					require.NoError(t, err)
					require.True(t, result.Healthy, result.Detail)
				})
			}
		})
	}
}

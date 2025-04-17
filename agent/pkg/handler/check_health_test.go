package handler_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/yusing/go-proxy/pkg/json"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/agent/pkg/handler"
	"github.com/yusing/go-proxy/internal/watcher/health"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestCheckHealthHTTP(t *testing.T) {
	tests := []struct {
		name            string
		setupServer     func() *httptest.Server
		queryParams     map[string]string
		expectedStatus  int
		expectedHealthy bool
	}{
		{
			name: "Valid",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
			queryParams: map[string]string{
				"scheme": "http",
				"host":   "localhost",
				"path":   "/",
			},
			expectedStatus:  http.StatusOK,
			expectedHealthy: true,
		},
		{
			name:        "InvalidQuery",
			setupServer: nil,
			queryParams: map[string]string{
				"scheme": "http",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:        "ConnectionError",
			setupServer: nil,
			queryParams: map[string]string{
				"scheme": "http",
				"host":   "localhost:12345",
			},
			expectedStatus:  http.StatusOK,
			expectedHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.setupServer != nil {
				server = tt.setupServer()
				defer server.Close()
				u, _ := url.Parse(server.URL)
				tt.queryParams["scheme"] = u.Scheme
				tt.queryParams["host"] = u.Host
				tt.queryParams["path"] = u.Path
			}

			recorder := httptest.NewRecorder()
			query := url.Values{}
			for key, value := range tt.queryParams {
				query.Set(key, value)
			}
			request := httptest.NewRequest(http.MethodGet, agent.APIEndpointBase+agent.EndpointHealth+"?"+query.Encode(), nil)
			handler.CheckHealth(recorder, request)

			expect.Equal(t, recorder.Code, tt.expectedStatus)

			if tt.expectedStatus == http.StatusOK {
				var result health.HealthCheckResult
				expect.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
				expect.Equal(t, result.Healthy, tt.expectedHealthy)
			}
		})
	}
}

func TestCheckHealthFileServer(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		expectedStatus  int
		expectedHealthy bool
		expectedDetail  string
	}{
		{
			name:            "ValidPath",
			path:            t.TempDir(),
			expectedStatus:  http.StatusOK,
			expectedHealthy: true,
			expectedDetail:  "",
		},
		{
			name:            "InvalidPath",
			path:            "/invalid",
			expectedStatus:  http.StatusOK,
			expectedHealthy: false,
			expectedDetail:  "stat /invalid: no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := url.Values{}
			query.Set("scheme", "fileserver")
			query.Set("path", tt.path)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, agent.APIEndpointBase+agent.EndpointHealth+"?"+query.Encode(), nil)
			handler.CheckHealth(recorder, request)

			expect.Equal(t, recorder.Code, tt.expectedStatus)

			var result health.HealthCheckResult
			expect.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
			expect.Equal(t, result.Healthy, tt.expectedHealthy)
			expect.Equal(t, result.Detail, tt.expectedDetail)
		})
	}
}

func TestCheckHealthTCPUDP(t *testing.T) {
	tcp, err := net.Listen("tcp", "localhost:0")
	expect.NoError(t, err)
	go func() {
		conn, err := tcp.Accept()
		expect.NoError(t, err)
		conn.Close()
	}()

	udp, err := net.ListenPacket("udp", "localhost:0")
	expect.NoError(t, err)
	go func() {
		buf := make([]byte, 1024)
		n, addr, err := udp.ReadFrom(buf)
		expect.NoError(t, err)
		expect.Equal(t, string(buf[:n]), "ping")
		_, _ = udp.WriteTo([]byte("pong"), addr)
		udp.Close()
	}()

	tests := []struct {
		name            string
		scheme          string
		host            string
		port            int
		expectedStatus  int
		expectedHealthy bool
	}{
		{
			name:            "ValidTCP",
			scheme:          "tcp",
			host:            "localhost",
			port:            tcp.Addr().(*net.TCPAddr).Port,
			expectedStatus:  http.StatusOK,
			expectedHealthy: true,
		},
		{
			name:            "InvalidHost",
			scheme:          "tcp",
			host:            "invalid",
			port:            8080,
			expectedStatus:  http.StatusOK,
			expectedHealthy: false,
		},
		{
			name:            "ValidUDP",
			scheme:          "udp",
			host:            "localhost",
			port:            udp.LocalAddr().(*net.UDPAddr).Port,
			expectedStatus:  http.StatusOK,
			expectedHealthy: true,
		},
		{
			name:            "InvalidHost",
			scheme:          "udp",
			host:            "invalid",
			port:            8080,
			expectedStatus:  http.StatusOK,
			expectedHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := url.Values{}
			query.Set("scheme", tt.scheme)
			query.Set("host", tt.host)
			query.Set("port", strconv.Itoa(tt.port))

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, agent.APIEndpointBase+agent.EndpointHealth+"?"+query.Encode(), nil)
			handler.CheckHealth(recorder, request)

			expect.Equal(t, recorder.Code, tt.expectedStatus)

			var result health.HealthCheckResult
			expect.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
			expect.Equal(t, result.Healthy, tt.expectedHealthy)
		})
	}
}

package entrypoint

import (
	"bufio"
	"crypto/tls"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	"github.com/stretchr/testify/require"
	agentcert "github.com/yusing/godoxy/agent/pkg/agent"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/task"
)

func TestHTTPServerLegacyProxyProtocolAcceptsDirectClient(t *testing.T) {
	ep := NewTestEntrypoint(t, &Config{SupportProxyProtocol: true})
	srv := newHTTPServer(ep)
	addr := reserveSNIListenAddr(t)
	require.NoError(t, srv.Listen(addr, HTTPProtoHTTP))
	t.Cleanup(srv.Close)

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + addr)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSharedHTTPSLegacyProxyProtocolAcceptsDirectClient(t *testing.T) {
	previousSNIRouting := common.SNIRoutingForTCPRoutes
	common.SNIRoutingForTCPRoutes = true
	t.Cleanup(func() { common.SNIRoutingForTCPRoutes = previousSNIRouting })

	_, serverAgent, _, err := agentcert.NewAgent()
	require.NoError(t, err)
	serverCert, err := serverAgent.ToTLSCert()
	require.NoError(t, err)
	autocert.SetCtx(task.GetTestTask(t), &staticCertProvider{cert: serverCert})

	ep := NewTestEntrypoint(t, &Config{SupportProxyProtocol: true})
	srv := newHTTPServer(ep)
	addr := reserveSNIListenAddr(t)
	require.NoError(t, srv.Listen(addr, HTTPProtoHTTPS))
	t.Cleanup(srv.Close)

	client := &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		}},
	}
	resp, err := client.Get("https://" + addr)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHTTPServerTrustedProxyRequiresAndAcceptsHeader(t *testing.T) {
	ep := NewTestEntrypoint(t, &Config{ProxyProtocol: &server.ProxyProtocolConfig{
		Mode:           server.ProxyProtocolModeRequired,
		TrustedProxies: []string{"127.0.0.1"},
	}})
	srv := newHTTPServer(ep)
	addr := reserveSNIListenAddr(t)
	require.NoError(t, srv.Listen(addr, HTTPProtoHTTP))
	t.Cleanup(srv.Close)

	client := &http.Client{Timeout: time.Second}
	_, err := client.Get("http://" + addr)
	require.Error(t, err, "trusted peer without a PROXY header must be rejected")

	conn, err := net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(time.Second)))

	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	destination, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("127.0.0.1", port))
	require.NoError(t, err)
	header := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        &net.TCPAddr{IP: net.ParseIP("203.0.113.25"), Port: 43210},
		DestinationAddr:   destination,
	}
	_, err = header.WriteTo(conn)
	require.NoError(t, err)
	_, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: example.test\r\nConnection: close\r\n\r\n"))
	require.NoError(t, err)

	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHTTPServerMixedProxyProtocolAcceptsUntrustedDirectClient(t *testing.T) {
	ep := NewTestEntrypoint(t, &Config{ProxyProtocol: &server.ProxyProtocolConfig{
		Mode:           server.ProxyProtocolModeMixed,
		TrustedProxies: []string{"192.0.2.1"},
	}})
	srv := newHTTPServer(ep)
	addr := reserveSNIListenAddr(t)
	require.NoError(t, srv.Listen(addr, HTTPProtoHTTP))
	t.Cleanup(srv.Close)

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + addr)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDurableProxyProtocolConfigPrecedesLegacyFlag(t *testing.T) {
	ep := NewTestEntrypoint(t, &Config{
		SupportProxyProtocol: true,
		ProxyProtocol: &server.ProxyProtocolConfig{
			Mode: server.ProxyProtocolModeDisabled,
		},
	})
	policy, err := ep.ProxyProtocolPolicy()
	require.NoError(t, err)
	require.False(t, policy.Enabled())
}

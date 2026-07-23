package proxmox

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewProxmoxHTTPClientHasBoundedCapacityAndTimeouts(t *testing.T) {
	client := newProxmoxHTTPClient(false)

	require.Equal(t, RequestTimeout, client.Timeout)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Equal(t, proxmoxMaxConnsPerHost, transport.MaxConnsPerHost)
	require.Equal(t, proxmoxMaxConnsPerHost, transport.MaxIdleConns)
	require.Equal(t, proxmoxMaxConnsPerHost, transport.MaxIdleConnsPerHost)
	require.Equal(t, RequestTimeout, transport.ResponseHeaderTimeout)
	require.Greater(t, transport.MaxConnsPerHost, maxConcurrentResourceLookups)
	require.Nil(t, transport.TLSClientConfig)

	insecureClient := newProxmoxHTTPClient(true)
	insecureTransport, ok := insecureClient.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, insecureTransport.TLSClientConfig)
	require.True(t, insecureTransport.TLSClientConfig.InsecureSkipVerify)
}

func TestProxmoxHTTPClientTimeoutReleasesConnection(t *testing.T) {
	releaseStalledRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stall" {
			select {
			case <-r.Context().Done():
			case <-releaseStalledRequest:
			}
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(func() {
		close(releaseStalledRequest)
		server.Close()
	})

	client := newProxmoxHTTPClient(false)
	client.Timeout = 250 * time.Millisecond
	transport := client.Transport.(*http.Transport)
	transport.Proxy = nil
	transport.MaxConnsPerHost = 1
	transport.MaxIdleConns = 1
	transport.MaxIdleConnsPerHost = 1
	transport.ResponseHeaderTimeout = 250 * time.Millisecond

	response, err := client.Get(server.URL + "/stall")
	require.Nil(t, response)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	response, err = client.Get(server.URL + "/healthy")
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "ok", string(body))
}

func TestProxmoxConnectionLimitDoesNotCollideAcrossHosts(t *testing.T) {
	stalledRequestStarted := make(chan struct{})
	stalledServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(stalledRequestStarted)
		<-r.Context().Done()
	}))
	t.Cleanup(stalledServer.Close)

	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(healthyServer.Close)

	client := newProxmoxHTTPClient(false)
	client.Timeout = 2 * time.Second
	transport := client.Transport.(*http.Transport)
	transport.Proxy = nil
	transport.MaxConnsPerHost = 1
	transport.MaxIdleConns = 1
	transport.MaxIdleConnsPerHost = 1
	transport.ResponseHeaderTimeout = 2 * time.Second

	stalledCtx, cancelStalled := context.WithCancel(t.Context())
	stalledErr := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(stalledCtx, http.MethodGet, stalledServer.URL, nil)
		if err != nil {
			stalledErr <- err
			return
		}
		response, err := client.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		stalledErr <- err
	}()

	select {
	case <-stalledRequestStarted:
	case <-time.After(time.Second):
		t.Fatal("stalled request did not reach its host")
	}

	response, err := client.Get(healthyServer.URL)
	require.NoError(t, err)
	_ = response.Body.Close()

	cancelStalled()
	require.ErrorIs(t, <-stalledErr, context.Canceled)
}

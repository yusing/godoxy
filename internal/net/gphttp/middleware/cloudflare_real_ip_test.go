package middleware

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	expect "github.com/yusing/goutils/testing"
)

func TestCloudflareRealIPSkipsFetchWithoutHeader(t *testing.T) {
	resetCloudflareRealIPTestState(t)

	loadCalls := 0
	loadCloudflareCIDRs = func() ([]*nettypes.CIDR, error) {
		loadCalls++
		return localCIDRs, nil
	}

	result, err := newMiddlewareTest(CloudflareRealIP, &testArgs{
		headers: http.Header{},
	})
	expect.NoError(t, err)
	expect.Equal(t, loadCalls, 0)
	expect.Equal(t, result.RemoteAddr, "192.0.2.1:1234")
}

func TestCloudflareRealIPUsesBundledCIDRsOnColdStartWithoutBlockingRequests(t *testing.T) {
	resetCloudflareRealIPTestState(t)

	loadStarted := make(chan struct{}, 1)
	releaseLoad := make(chan struct{})
	var loadCalls atomic.Int64
	loadCloudflareCIDRs = func() ([]*nettypes.CIDR, error) {
		loadCalls.Add(1)
		loadStarted <- struct{}{}
		<-releaseLoad
		return localCIDRs, nil
	}

	first, err := newMiddlewareTest(CloudflareRealIP, &testArgs{
		remoteAddr: "127.0.0.1:1234",
		headers: http.Header{
			"CF-Connecting-IP": []string{"198.51.100.10"},
		},
	})
	expect.NoError(t, err)
	require.NotEmpty(t, cfCIDRs.Load())
	expect.Equal(t, first.RemoteAddr, "198.51.100.10")
	require.Eventually(t, func() bool {
		return loadCalls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	<-loadStarted

	second, err := newMiddlewareTest(CloudflareRealIP, &testArgs{
		remoteAddr: "127.0.0.1:1234",
		headers: http.Header{
			"CF-Connecting-IP": []string{"198.51.100.11"},
		},
	})
	expect.NoError(t, err)
	expect.Equal(t, loadCalls.Load(), int64(1))
	expect.Equal(t, second.RemoteAddr, "198.51.100.11")
	close(releaseLoad)
}

func TestCloudflareRealIPRefreshesStaleCIDRsInBackground(t *testing.T) {
	resetCloudflareRealIPTestState(t)

	now := time.Unix(1_700_000_000, 0)
	timeNow = func() time.Time { return now }

	oldCIDRValue, err := nettypes.ParseCIDR("127.0.0.1/32")
	require.NoError(t, err)
	oldCIDR := &oldCIDRValue
	newCIDRValue, err := nettypes.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)
	newCIDR := &newCIDRValue
	cfCIDRs.Store([]*nettypes.CIDR{oldCIDR})
	cfCIDRsLastUpdate.Store(now.Add(-cfCIDRsUpdateInterval - time.Second))

	loadStarted := make(chan struct{}, 1)
	releaseLoad := make(chan struct{})
	var loadCalls atomic.Int64
	loadCloudflareCIDRs = func() ([]*nettypes.CIDR, error) {
		loadCalls.Add(1)
		loadStarted <- struct{}{}
		<-releaseLoad
		return []*nettypes.CIDR{newCIDR}, nil
	}

	first, err := newMiddlewareTest(CloudflareRealIP, &testArgs{
		remoteAddr: "127.0.0.1:1234",
		headers: http.Header{
			"CF-Connecting-IP": []string{"198.51.100.20"},
		},
	})
	expect.NoError(t, err)
	expect.Equal(t, first.RemoteAddr, "198.51.100.20")
	require.Eventually(t, func() bool {
		return loadCalls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	<-loadStarted

	second, err := newMiddlewareTest(CloudflareRealIP, &testArgs{
		remoteAddr: "127.0.0.1:1234",
		headers: http.Header{
			"CF-Connecting-IP": []string{"198.51.100.21"},
		},
	})
	expect.NoError(t, err)
	expect.Equal(t, second.RemoteAddr, "198.51.100.21")
	expect.Equal(t, loadCalls.Load(), int64(1))

	close(releaseLoad)
	require.Eventually(t, func() bool {
		cidrs := cfCIDRs.Load()
		return len(cidrs) == 1 && cidrs[0] == newCIDR
	}, time.Second, 10*time.Millisecond)
}

func TestCloudflareRealIPDoesNotBlockRequestsWhileRefreshIsStuck(t *testing.T) {
	resetCloudflareRealIPTestState(t)

	loadStarted := make(chan struct{}, 1)
	releaseLoad := make(chan struct{})
	loadCloudflareCIDRs = func() ([]*nettypes.CIDR, error) {
		loadStarted <- struct{}{}
		<-releaseLoad
		return localCIDRs, nil
	}

	resultCh := make(chan *TestResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := newMiddlewareTest(CloudflareRealIP, &testArgs{
			remoteAddr: "127.0.0.1:1234",
			headers: http.Header{
				"CF-Connecting-IP": []string{"198.51.100.30"},
			},
		})
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case result := <-resultCh:
		expect.Equal(t, result.RemoteAddr, "198.51.100.30")
	case <-time.After(250 * time.Millisecond):
		t.Fatal("request blocked on Cloudflare CIDR refresh")
	}

	<-loadStarted
	close(releaseLoad)
}

func TestCloudflareRealIPFailureBacksOffAndKeepsServing(t *testing.T) {
	resetCloudflareRealIPTestState(t)

	now := time.Unix(1_700_000_000, 0)
	timeNow = func() time.Time { return now }

	var loadCalls atomic.Int64
	loadCloudflareCIDRs = func() ([]*nettypes.CIDR, error) {
		loadCalls.Add(1)
		return nil, errors.New("timeout")
	}

	first, err := newMiddlewareTest(CloudflareRealIP, &testArgs{
		remoteAddr: "127.0.0.1:1234",
		headers: http.Header{
			"CF-Connecting-IP": []string{"198.51.100.12"},
		},
	})
	expect.NoError(t, err)
	require.Eventually(t, func() bool {
		return loadCalls.Load() == 1 && !cfCIDRsNextRetry.Load().IsZero()
	}, time.Second, 10*time.Millisecond)
	expect.Equal(t, first.RemoteAddr, "198.51.100.12")

	second, err := newMiddlewareTest(CloudflareRealIP, &testArgs{
		remoteAddr: "127.0.0.1:1234",
		headers: http.Header{
			"CF-Connecting-IP": []string{"198.51.100.13"},
		},
	})
	expect.NoError(t, err)
	expect.Equal(t, loadCalls.Load(), int64(1))
	expect.Equal(t, second.RemoteAddr, "198.51.100.13")

	now = now.Add(cfCIDRsUpdateRetryInterval + time.Millisecond)

	third, err := newMiddlewareTest(CloudflareRealIP, &testArgs{
		remoteAddr: "127.0.0.1:1234",
		headers: http.Header{
			"CF-Connecting-IP": []string{"198.51.100.14"},
		},
	})
	expect.NoError(t, err)
	require.Eventually(t, func() bool {
		return loadCalls.Load() == 2
	}, time.Second, 10*time.Millisecond)
	expect.Equal(t, third.RemoteAddr, "198.51.100.14")
}

func resetCloudflareRealIPTestState(t *testing.T) {
	t.Helper()

	waitForCloudflareRefreshIdle(t)

	prevCIDRs := cfCIDRs.Load()
	prevLastUpdate := cfCIDRsLastUpdate.Load()
	prevNextRetry := cfCIDRsNextRetry.Load()
	prevRefreshing := cfCIDRsRefreshing.Load()
	prevLoad := loadCloudflareCIDRs
	prevUseLocalCIDRs := cloudflareRealIPUseLocalCIDRs
	prevTimeNow := timeNow

	cfCIDRs.Store(nil)
	cfCIDRsLastUpdate.Store(time.Time{})
	cfCIDRsNextRetry.Store(time.Time{})
	cfCIDRsRefreshing.Store(false)
	loadCloudflareCIDRs = func() ([]*nettypes.CIDR, error) {
		return localCIDRs, nil
	}
	cloudflareRealIPUseLocalCIDRs = func() bool {
		return false
	}
	timeNow = time.Now

	t.Cleanup(func() {
		waitForCloudflareRefreshIdle(t)
		cfCIDRs.Store(prevCIDRs)
		cfCIDRsLastUpdate.Store(prevLastUpdate)
		cfCIDRsNextRetry.Store(prevNextRetry)
		cfCIDRsRefreshing.Store(prevRefreshing)
		loadCloudflareCIDRs = prevLoad
		cloudflareRealIPUseLocalCIDRs = prevUseLocalCIDRs
		timeNow = prevTimeNow
	})
}

func waitForCloudflareRefreshIdle(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		return !cfCIDRsRefreshing.Load()
	}, time.Second, 10*time.Millisecond)
}

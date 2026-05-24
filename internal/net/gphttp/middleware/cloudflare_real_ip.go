package middleware

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/synk"
)

type cloudflareRealIP struct {
	realIP realIP
}

const (
	cfIPv4CIDRsEndpoint        = "https://www.cloudflare.com/ips-v4"
	cfIPv6CIDRsEndpoint        = "https://www.cloudflare.com/ips-v6"
	cfCIDRsUpdateInterval      = time.Hour
	cfCIDRsUpdateRetryInterval = 3 * time.Second
)

var (
	cfCIDRs           synk.Value[[]*nettypes.CIDR]
	cfCIDRsLastUpdate synk.Value[time.Time]
	cfCIDRsNextRetry  synk.Value[time.Time]
	cfCIDRsMu         sync.Mutex
	cfCIDRsRefreshing atomic.Bool

	// RFC 1918.
	localCIDRs = []*nettypes.CIDR{
		{IP: net.IPv4(127, 0, 0, 1), Mask: net.IPv4Mask(255, 255, 255, 255)}, // 127.0.0.1/32
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.IPv4Mask(255, 0, 0, 0)},        // 10.0.0.0/8
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.IPv4Mask(255, 240, 0, 0)},    // 172.16.0.0/12
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.IPv4Mask(255, 255, 0, 0)},   // 192.168.0.0/16
	}

	loadSeedCloudflareCIDRs = sync.OnceValue(mustLoadSeedCloudflareCIDRs)

	loadCloudflareCIDRs = func() ([]*nettypes.CIDR, error) {
		ipv4CIDRs, ipv4Err := fetchCloudflareCIDRs(cfIPv4CIDRsEndpoint)
		ipv6CIDRs, ipv6Err := fetchCloudflareCIDRs(cfIPv6CIDRsEndpoint)
		if err := errors.Join(ipv4Err, ipv6Err); err != nil {
			return nil, err
		}
		cidrs := make([]*nettypes.CIDR, 0, len(ipv4CIDRs)+len(ipv6CIDRs)+len(localCIDRs))
		cidrs = append(cidrs, ipv4CIDRs...)
		cidrs = append(cidrs, ipv6CIDRs...)
		cidrs = append(cidrs, localCIDRs...)
		return cidrs, nil
	}
	cloudflareRealIPUseLocalCIDRs = func() bool {
		return common.IsTest
	}
	timeNow = time.Now
)

var CloudflareRealIP = NewMiddleware[cloudflareRealIP]()

// setup implements MiddlewareWithSetup.
func (cri *cloudflareRealIP) setup() {
	ensureCloudflareCIDRsSeeded()
	cri.realIP = realIP{
		Header:    "CF-Connecting-IP",
		From:      cfCIDRs.Load(),
		Recursive: true,
	}
}

// before implements RequestModifier.
func (cri *cloudflareRealIP) before(w http.ResponseWriter, r *http.Request) bool {
	if len(r.Header.Values(cri.realIP.Header)) == 0 && len(r.Header[cri.realIP.Header]) == 0 {
		return cri.realIP.before(w, r)
	}
	cidrs := tryFetchCFCIDR()
	if cidrs != nil {
		cri.realIP.From = cidrs
	}
	return cri.realIP.before(w, r)
}

func tryFetchCFCIDR() []*nettypes.CIDR {
	cachedCIDRs := cfCIDRs.Load()
	if len(cachedCIDRs) == 0 {
		ensureCloudflareCIDRsSeeded()
		cachedCIDRs = cfCIDRs.Load()
		if len(cachedCIDRs) == 0 {
			return nil
		}
	}
	now := timeNow()
	lastUpdate := cfCIDRsLastUpdate.Load()
	if !lastUpdate.IsZero() && now.Sub(lastUpdate) < cfCIDRsUpdateInterval {
		return cachedCIDRs
	}
	if nextRetry := cfCIDRsNextRetry.Load(); !nextRetry.IsZero() && now.Before(nextRetry) {
		return cachedCIDRs
	}
	refreshCloudflareCIDRs()
	return cachedCIDRs
}

func ensureCloudflareCIDRsSeeded() {
	if len(cfCIDRs.Load()) > 0 {
		return
	}

	cfCIDRsMu.Lock()
	defer cfCIDRsMu.Unlock()

	if len(cfCIDRs.Load()) > 0 {
		return
	}

	cfCIDRs.Store(cloneCloudflareCIDRs(loadSeedCloudflareCIDRs()))
	cfCIDRsLastUpdate.Store(time.Time{})
	cfCIDRsNextRetry.Store(time.Time{})
}

func refreshCloudflareCIDRs() {
	if !cfCIDRsRefreshing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer cfCIDRsRefreshing.Store(false)

		cfCIDRsMu.Lock()
		defer cfCIDRsMu.Unlock()

		refreshCloudflareCIDRsLocked()
	}()
}

func refreshCloudflareCIDRsLocked() []*nettypes.CIDR {
	now := timeNow()
	current := cfCIDRs.Load()
	lastUpdate := cfCIDRsLastUpdate.Load()
	if len(current) > 0 && now.Sub(lastUpdate) < cfCIDRsUpdateInterval {
		return current
	}
	if nextRetry := cfCIDRsNextRetry.Load(); !nextRetry.IsZero() && now.Before(nextRetry) {
		return current
	}

	var updated []*nettypes.CIDR
	if cloudflareRealIPUseLocalCIDRs() {
		updated = cloneCloudflareCIDRs(localCIDRs)
	} else {
		var err error
		updated, err = loadCloudflareCIDRs()
		if err != nil {
			cfCIDRsNextRetry.Store(now.Add(cfCIDRsUpdateRetryInterval))
			log.Err(err).Msg("failed to update cloudflare range, retry in " + strutils.FormatDuration(cfCIDRsUpdateRetryInterval))
			return current
		}
		if len(updated) == 0 {
			log.Warn().Msg("cloudflare CIDR range is empty")
		}
	}

	cfCIDRs.Store(cloneCloudflareCIDRs(updated))
	cfCIDRsLastUpdate.Store(now)
	cfCIDRsNextRetry.Store(time.Time{})
	log.Info().Msg("cloudflare CIDR range updated")
	return updated
}

func mustLoadSeedCloudflareCIDRs() []*nettypes.CIDR {
	if len(generatedCloudflareCIDRsRaw) == 0 {
		panic("missing generated Cloudflare CIDR snapshot; run `make gen-cloudflare-cidrs`")
	}
	cidrs, err := parseCloudflareCIDRs(generatedCloudflareCIDRsRaw)
	if err != nil {
		panic(err)
	}
	cidrs = append(cidrs, localCIDRs...)
	return cidrs
}

func fetchCloudflareCIDRs(endpoint string) ([]*nettypes.CIDR, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseCloudflareCIDRs(body)
}

func parseCloudflareCIDRs(body []byte) ([]*nettypes.CIDR, error) {
	cfCIDRs := make([]*nettypes.CIDR, 0, bytes.Count(body, []byte{'\n'})+1)
	for line := range bytes.Lines(body) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		_, cidr, err := net.ParseCIDR(string(line))
		if err != nil {
			return nil, fmt.Errorf("cloudflare responded an invalid CIDR: %s", line)
		}

		cfCIDRs = append(cfCIDRs, (*nettypes.CIDR)(cidr))
	}
	return cfCIDRs, nil
}

func cloneCloudflareCIDRs(cidrs []*nettypes.CIDR) []*nettypes.CIDR {
	return slices.Clone(cidrs)
}

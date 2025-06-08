package middleware

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/common"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils/atomic"
	"github.com/yusing/go-proxy/internal/utils/strutils"
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
	cfCIDRsLastUpdate atomic.Value[time.Time]
	cfCIDRsMu         sync.Mutex

	// RFC 1918.
	localCIDRs = []*nettypes.CIDR{
		{IP: net.IPv4(127, 0, 0, 1), Mask: net.IPv4Mask(255, 255, 255, 255)}, // 127.0.0.1/32
		{IP: net.IPv4(10, 0, 0, 0), Mask: net.IPv4Mask(255, 0, 0, 0)},        // 10.0.0.0/8
		{IP: net.IPv4(172, 16, 0, 0), Mask: net.IPv4Mask(255, 240, 0, 0)},    // 172.16.0.0/12
		{IP: net.IPv4(192, 168, 0, 0), Mask: net.IPv4Mask(255, 255, 0, 0)},   // 192.168.0.0/16
	}
)

var CloudflareRealIP = NewMiddleware[cloudflareRealIP]()

// setup implements MiddlewareWithSetup.
func (cri *cloudflareRealIP) setup() {
	cri.realIP = realIP{
		Header:    "CF-Connecting-IP",
		Recursive: true,
	}
}

// before implements RequestModifier.
func (cri *cloudflareRealIP) before(w http.ResponseWriter, r *http.Request) bool {
	cidrs := tryFetchCFCIDR()
	if cidrs != nil {
		cri.realIP.From = cidrs
	}
	return cri.realIP.before(w, r)
}

func tryFetchCFCIDR() (cfCIDRs []*nettypes.CIDR) {
	if time.Since(cfCIDRsLastUpdate.Load()) < cfCIDRsUpdateInterval {
		return
	}

	cfCIDRsMu.Lock()
	defer cfCIDRsMu.Unlock()

	if time.Since(cfCIDRsLastUpdate.Load()) < cfCIDRsUpdateInterval {
		return
	}

	if common.IsTest {
		cfCIDRs = localCIDRs
	} else {
		cfCIDRs = make([]*nettypes.CIDR, 0, 30)
		err := errors.Join(
			fetchUpdateCFIPRange(cfIPv4CIDRsEndpoint, &cfCIDRs),
			fetchUpdateCFIPRange(cfIPv6CIDRsEndpoint, &cfCIDRs),
		)
		if err != nil {
			cfCIDRsLastUpdate.Store(time.Now().Add(-cfCIDRsUpdateRetryInterval - cfCIDRsUpdateInterval))
			log.Err(err).Msg("failed to update cloudflare range, retry in " + strutils.FormatDuration(cfCIDRsUpdateRetryInterval))
			return nil
		}
		if len(cfCIDRs) == 0 {
			log.Warn().Msg("cloudflare CIDR range is empty")
		}
	}

	cfCIDRsLastUpdate.Store(time.Now())
	log.Info().Msg("cloudflare CIDR range updated")
	return
}

func fetchUpdateCFIPRange(endpoint string, cfCIDRs *[]*nettypes.CIDR) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	for _, line := range strutils.SplitLine(string(body)) {
		if line == "" {
			continue
		}
		_, cidr, err := net.ParseCIDR(line)
		if err != nil {
			return fmt.Errorf("cloudflare responeded an invalid CIDR: %s", line)
		}

		*cfCIDRs = append(*cfCIDRs, (*nettypes.CIDR)(cidr))
	}
	*cfCIDRs = append(*cfCIDRs, localCIDRs...)
	return nil
}

package maxmind

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/goutils/task"
	"golang.org/x/time/rate"
)

var instance *MaxMind

var (
	warnOnce               sync.Once
	errLogRateLimiter      = rate.NewLimiter(rate.Every(3*time.Second), 1)
	errLogSuppressedCounts = xsync.NewMap[string, *atomic.Int64](xsync.WithPresize(32))
)

func warnNotConfigured() {
	log.Warn().Msg("MaxMind not configured, geo lookup will fail")
	notif.Notify(&notif.LogMessage{
		Level: zerolog.WarnLevel,
		Title: "MaxMind not configured",
		Body:  notif.MessageBody("MaxMind is not configured, geo lookup will fail"),
		Color: notif.ColorError,
	})
}

func SetInstance(parent task.Parent, cfg *Config) error {
	newInstance := &MaxMind{Config: cfg}
	if err := newInstance.LoadMaxMindDB(parent); err != nil {
		return err
	}
	instance = newInstance
	return nil
}

func HasInstance() bool {
	return instance != nil
}

func LookupCity(ip *IPInfo) (*City, bool) {
	if ip.City != nil {
		return ip.City, false
	}

	if instance == nil {
		warnOnce.Do(warnNotConfigured)
		return nil, false
	}

	city, err := lookupCityNoContext(ip.Str)
	if err != nil {
		logLookupCityError(ip.Str, err)
		return nil, false
	}
	ip.City = city
	return city, true
}

func lookupCityErrorKey(err error) string {
	return err.Error()
}

func incrementSuppressedLookupCityError(err error) {
	key := lookupCityErrorKey(err)
	counter, _ := errLogSuppressedCounts.LoadOrCompute(key, func() (*atomic.Int64, bool) {
		return &atomic.Int64{}, false
	})
	counter.Add(1)
}

func flushSuppressedLookupCityError(err error) int64 {
	counter, ok := errLogSuppressedCounts.Load(lookupCityErrorKey(err))
	if !ok {
		return 0
	}
	return counter.Swap(0)
}

func logLookupCityError(ipStr string, err error) {
	if !errLogRateLimiter.Allow() {
		incrementSuppressedLookupCityError(err)
		return
	}

	event := log.Err(err).Str("ip", ipStr)
	if suppressedCount := flushSuppressedLookupCityError(err); suppressedCount > 0 {
		event = event.Int64("suppressed_count", suppressedCount)
	}
	event.Msg("failed to lookup city")
}

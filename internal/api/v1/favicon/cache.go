package favicon

import (
	"encoding/json"
	"time"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/jsonstore"
	"github.com/yusing/go-proxy/internal/logging"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/task"
)

type cacheEntry struct {
	Icon       []byte    `json:"icon"`
	LastAccess time.Time `json:"lastAccess"`
}

// cache key can be absolute url or route name.
var iconCache = jsonstore.Store[*cacheEntry](common.NamespaceIconCache)

const (
	iconCacheTTL    = 3 * 24 * time.Hour
	cleanUpInterval = time.Hour
)

func init() {
	go func() {
		cleanupTicker := time.NewTicker(cleanUpInterval)
		defer cleanupTicker.Stop()
		for {
			select {
			case <-task.RootContextCanceled():
				return
			case <-cleanupTicker.C:
				pruneExpiredIconCache()
			}
		}
	}()
}

func pruneExpiredIconCache() {
	nPruned := 0
	for key, icon := range iconCache.Range {
		if icon.IsExpired() {
			iconCache.Delete(key)
			nPruned++
		}
	}
	if nPruned > 0 {
		logging.Info().Int("pruned", nPruned).Msg("pruned expired icon cache")
	}
}

func routeKey(r route.HTTPRoute) string {
	return r.ProviderName() + ":" + r.TargetName()
}

func PruneRouteIconCache(route route.HTTPRoute) {
	iconCache.Delete(routeKey(route))
}

func loadIconCache(key string) *fetchResult {
	icon, ok := iconCache.Load(key)
	if ok && icon != nil {
		logging.Debug().
			Str("key", key).
			Msg("icon found in cache")
		icon.LastAccess = time.Now()
		return &fetchResult{icon: icon.Icon}
	}
	return nil
}

func storeIconCache(key string, icon []byte) {
	iconCache.Store(key, &cacheEntry{Icon: icon, LastAccess: time.Now()})
}

func (e *cacheEntry) IsExpired() bool {
	return time.Since(e.LastAccess) > iconCacheTTL
}

func (e *cacheEntry) UnmarshalJSON(data []byte) error {
	attempt := struct {
		Icon       []byte    `json:"icon"`
		LastAccess time.Time `json:"lastAccess"`
	}{}
	err := json.Unmarshal(data, &attempt)
	if err == nil {
		e.Icon = attempt.Icon
		e.LastAccess = attempt.LastAccess
		return nil
	}
	// fallback to bytes
	err = json.Unmarshal(data, &e.Icon)
	if err == nil {
		e.LastAccess = time.Now()
		return nil
	}
	return err
}

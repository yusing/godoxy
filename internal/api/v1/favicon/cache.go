package favicon

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
)

type cacheEntry struct {
	Icon       []byte    `json:"icon"`
	LastAccess time.Time `json:"lastAccess"`
}

// cache key can be absolute url or route name.
var (
	iconCache   = make(map[string]*cacheEntry)
	iconCacheMu sync.RWMutex
)

const (
	iconCacheTTL    = 3 * 24 * time.Hour
	cleanUpInterval = time.Hour
)

func InitIconCache() {
	iconCacheMu.Lock()
	defer iconCacheMu.Unlock()

	err := utils.LoadJSONIfExist(common.IconCachePath, &iconCache)
	if err != nil {
		logging.Error().Err(err).Msg("failed to load icon cache")
	} else if len(iconCache) > 0 {
		logging.Info().Int("count", len(iconCache)).Msg("icon cache loaded")
	}

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

	task.OnProgramExit("save_favicon_cache", func() {
		iconCacheMu.Lock()
		defer iconCacheMu.Unlock()

		if len(iconCache) == 0 {
			return
		}

		if err := utils.SaveJSON(common.IconCachePath, &iconCache, 0o644); err != nil {
			logging.Error().Err(err).Msg("failed to save icon cache")
		}
	})
}

func pruneExpiredIconCache() {
	iconCacheMu.Lock()
	defer iconCacheMu.Unlock()

	nPruned := 0
	for key, icon := range iconCache {
		if icon.IsExpired() {
			delete(iconCache, key)
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
	iconCacheMu.Lock()
	defer iconCacheMu.Unlock()
	delete(iconCache, routeKey(route))
}

func loadIconCache(key string) *fetchResult {
	iconCacheMu.RLock()
	defer iconCacheMu.RUnlock()

	icon, ok := iconCache[key]
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
	iconCacheMu.Lock()
	defer iconCacheMu.Unlock()
	iconCache[key] = &cacheEntry{Icon: icon, LastAccess: time.Now()}
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

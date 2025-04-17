package homepage

import (
	"encoding/base64"
	"sync"
	"time"

	"github.com/yusing/go-proxy/pkg/json"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/atomic"
)

type cacheEntry struct {
	Icon        []byte                  `json:"icon"`
	ContentType string                  `json:"content_type"`
	LastAccess  atomic.Value[time.Time] `json:"last_access"`
}

// cache key can be absolute url or route name.
var (
	iconCache   = make(map[string]*cacheEntry)
	iconCacheMu sync.RWMutex
)

const (
	iconCacheTTL    = 3 * 24 * time.Hour
	cleanUpInterval = time.Minute
	maxCacheSize    = 1024 * 1024 // 1MB
	maxCacheEntries = 100
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
	if len(iconCache) > maxCacheEntries {
		newIconCache := make(map[string]*cacheEntry, maxCacheEntries)
		i := 0
		for key, icon := range iconCache {
			if i == maxCacheEntries {
				break
			}
			if !icon.IsExpired() {
				newIconCache[key] = icon
				i++
			}
		}
		iconCache = newIconCache
	}
	if nPruned > 0 {
		logging.Info().Int("pruned", nPruned).Msg("pruned expired icon cache")
	}
}

func routeKey(r route) string {
	return r.ProviderName() + ":" + r.TargetName()
}

func PruneRouteIconCache(route route) {
	iconCacheMu.Lock()
	defer iconCacheMu.Unlock()
	delete(iconCache, routeKey(route))
}

func loadIconCache(key string) *FetchResult {
	iconCacheMu.RLock()
	defer iconCacheMu.RUnlock()

	icon, ok := iconCache[key]
	if ok && len(icon.Icon) > 0 {
		logging.Debug().
			Str("key", key).
			Msg("icon found in cache")
		icon.LastAccess.Store(time.Now())
		return &FetchResult{Icon: icon.Icon, contentType: icon.ContentType}
	}
	return nil
}

func storeIconCache(key string, result *FetchResult) {
	icon := result.Icon
	if len(icon) > maxCacheSize {
		logging.Debug().Int("size", len(icon)).Msg("icon cache size exceeds max cache size")
		return
	}
	iconCacheMu.Lock()
	defer iconCacheMu.Unlock()
	entry := &cacheEntry{Icon: icon, ContentType: result.contentType}
	entry.LastAccess.Store(time.Now())
	iconCache[key] = entry
	logging.Debug().Str("key", key).Int("size", len(icon)).Msg("stored icon cache")
}

func (e *cacheEntry) IsExpired() bool {
	return time.Since(e.LastAccess.Load()) > iconCacheTTL
}

func (e *cacheEntry) UnmarshalJSON(data []byte) error {
	// check if data is json
	if json.Valid(data) {
		err := json.Unmarshal(data, &e)
		// return only if unmarshal is successful
		// otherwise fallback to base64
	if err == nil {
		return nil
		}
	}
	// fallback to base64
	icon, err := base64.StdEncoding.DecodeString(string(data))
	if err == nil {
		e.Icon = icon
		e.LastAccess.Store(time.Now())
		return nil
	}
	return err
}

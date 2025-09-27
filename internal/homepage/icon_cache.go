package homepage

import (
	"encoding/base64"
	"encoding/json"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/jsonstore"
	"github.com/yusing/godoxy/internal/utils"
	"github.com/yusing/godoxy/internal/utils/atomic"
	"github.com/yusing/goutils/task"
)

type cacheEntry struct {
	Icon        []byte                  `json:"icon"`
	ContentType string                  `json:"content_type,omitempty"`
	LastAccess  atomic.Value[time.Time] `json:"last_access"`
}

// cache key can be absolute url or route name.
var (
	iconCache = jsonstore.Store[*cacheEntry](common.NamespaceIconCache)
	iconMu    sync.RWMutex
)

const (
	iconCacheTTL    = 3 * 24 * time.Hour
	cleanUpInterval = time.Minute
	maxIconSize     = 1024 * 1024 // 1MB
	maxCacheEntries = 100
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
	if iconCache.Size() > maxCacheEntries {
		iconCache.Clear()
		newIconCache := make(map[string]*cacheEntry, maxCacheEntries)
		i := 0
		for key, icon := range iconCache.Range {
			if i == maxCacheEntries {
				break
			}
			if !icon.IsExpired() {
				newIconCache[key] = icon
				i++
			}
		}
		for key, icon := range newIconCache {
			iconCache.Store(key, icon)
		}
	}
	if nPruned > 0 {
		log.Info().Int("pruned", nPruned).Msg("pruned expired icon cache")
	}
}

func PruneRouteIconCache(route route) {
	iconCache.Delete(route.Key())
}

func loadIconCache(key string) *FetchResult {
	iconMu.RLock()
	defer iconMu.RUnlock()
	icon, ok := iconCache.Load(key)
	if ok && len(icon.Icon) > 0 {
		log.Debug().
			Str("key", key).
			Msg("icon found in cache")
		icon.LastAccess.Store(utils.TimeNow())
		return &FetchResult{Icon: icon.Icon, contentType: icon.ContentType}
	}
	return nil
}

func storeIconCache(key string, result *FetchResult) {
	icon := result.Icon
	if len(icon) > maxIconSize {
		log.Debug().Int("size", len(icon)).Msg("icon cache size exceeds max cache size")
		return
	}

	iconMu.Lock()
	defer iconMu.Unlock()

	entry := &cacheEntry{Icon: icon, ContentType: result.contentType}
	entry.LastAccess.Store(time.Now())
	iconCache.Store(key, entry)
	log.Debug().Str("key", key).Int("size", len(icon)).Msg("stored icon cache")
}

func (e *cacheEntry) IsExpired() bool {
	return time.Since(e.LastAccess.Load()) > iconCacheTTL
}

func (e *cacheEntry) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Icon        []byte    `json:"icon"`
		ContentType string    `json:"content_type,omitempty"`
		LastAccess  time.Time `json:"last_access"`
	}
	// check if data is json
	if json.Valid(data) {
		err := json.Unmarshal(data, &tmp)
		// return only if unmarshal is successful
		// otherwise fallback to base64
		if err == nil {
			e.Icon = tmp.Icon
			e.ContentType = tmp.ContentType
			e.LastAccess.Store(tmp.LastAccess)
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

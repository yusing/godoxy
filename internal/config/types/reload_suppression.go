package config

import (
	"sync"
	"time"

	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
)

const configReloadSuppressionTTL = 5 * time.Second

var (
	configReloadSuppressionMu        sync.Mutex
	configReloadSuppressionExpiresAt time.Time
)

func SuppressNextConfigReload() {
	SuppressNextConfigReloadUntil(time.Now().Add(configReloadSuppressionTTL))
}

func SuppressNextConfigReloadUntil(expiresAt time.Time) {
	configReloadSuppressionMu.Lock()
	defer configReloadSuppressionMu.Unlock()

	configReloadSuppressionExpiresAt = expiresAt
}

func ClearConfigReloadSuppression() {
	configReloadSuppressionMu.Lock()
	defer configReloadSuppressionMu.Unlock()

	configReloadSuppressionExpiresAt = time.Time{}
}

func ConsumeSuppressedConfigReload(ev []watcherEvents.Event, configFilename string) bool {
	configReloadSuppressionMu.Lock()
	defer configReloadSuppressionMu.Unlock()

	if configReloadSuppressionExpiresAt.IsZero() {
		return false
	}
	if time.Now().After(configReloadSuppressionExpiresAt) {
		configReloadSuppressionExpiresAt = time.Time{}
		return false
	}
	for _, event := range ev {
		if event.ActorName != configFilename {
			continue
		}
		switch event.Action {
		case watcherEvents.ActionFileWritten, watcherEvents.ActionFileCreated:
			configReloadSuppressionExpiresAt = time.Time{}
			return true
		}
	}
	return false
}

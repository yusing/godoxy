package jsonstore

import (
	"encoding/json"
	"path/filepath"
	"sync"

	"github.com/puzpuzpuz/xsync/v3"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
)

type jsonStoreInternal struct{ *xsync.MapOf[string, any] }
type namespace string

var stores = make(map[namespace]jsonStoreInternal)
var storesMu sync.Mutex
var storesPath = filepath.Join(common.DataDir, "data.json")

func Initialize() {
	if err := load(); err != nil {
		logging.Error().Err(err).Msg("failed to load stores")
	}

	task.OnProgramExit("save_stores", func() {
		if err := save(); err != nil {
			logging.Error().Err(err).Msg("failed to save stores")
		}
	})
}

func load() error {
	storesMu.Lock()
	defer storesMu.Unlock()
	if err := utils.LoadJSONIfExist(storesPath, &stores); err != nil {
		return err
	}
	return nil
}

func save() error {
	storesMu.Lock()
	defer storesMu.Unlock()
	return utils.SaveJSON(storesPath, &stores, 0o644)
}

func (s jsonStoreInternal) MarshalJSON() ([]byte, error) {
	return json.Marshal(xsync.ToPlainMapOf(s.MapOf))
}

func (s jsonStoreInternal) UnmarshalJSON(data []byte) error {
	var tmp map[string]any
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	s.MapOf = xsync.NewMapOf[string, any](xsync.WithPresize(len(tmp)))
	for k, v := range tmp {
		s.MapOf.Store(k, v)
	}
	return nil
}

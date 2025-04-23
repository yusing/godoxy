package jsonstore

import (
	"encoding/json"
	"path/filepath"
	"sync"

	"github.com/puzpuzpuz/xsync/v3"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
)

type namespace string

type Typed[VT any] struct {
	*xsync.MapOf[string, VT]
}

type storesMap struct {
	sync.RWMutex
	m map[namespace]any
}

var stores = storesMap{m: make(map[namespace]any)}
var storesPath = common.DataDir

func init() {
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
	stores.Lock()
	defer stores.Unlock()
	errs := gperr.NewBuilder("failed to load data stores")
	for ns, store := range stores.m {
		if err := utils.LoadJSONIfExist(filepath.Join(storesPath, string(ns)+".json"), &store); err != nil {
			errs.Add(err)
		}
	}
	return errs.Error()
}

func save() error {
	stores.Lock()
	defer stores.Unlock()
	errs := gperr.NewBuilder("failed to save data stores")
	for ns, store := range stores.m {
		if err := utils.SaveJSON(filepath.Join(common.DataDir, string(ns)+".json"), &store, 0o644); err != nil {
			errs.Add(err)
		}
	}
	return errs.Error()
}

func Store[VT any](namespace namespace) Typed[VT] {
	stores.Lock()
	defer stores.Unlock()
	if s, ok := stores.m[namespace]; ok {
		return s.(Typed[VT])
	}
	m := Typed[VT]{MapOf: xsync.NewMapOf[string, VT]()}
	stores.m[namespace] = m
	return m
}

func (s Typed[VT]) MarshalJSON() ([]byte, error) {
	tmp := make(map[string]VT, s.Size())
	for k, v := range s.Range {
		tmp[k] = v
	}
	return json.Marshal(tmp)
}

func (s Typed[VT]) UnmarshalJSON(data []byte) error {
	tmp := make(map[string]VT)
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	s.MapOf = xsync.NewMapOf[string, VT](xsync.WithPresize(len(tmp)))
	for k, v := range tmp {
		s.MapOf.Store(k, v)
	}
	return nil
}

package jsonstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"

	"maps"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
)

type namespace string

type MapStore[VT any] struct {
	*xsync.Map[string, VT]
}

type ObjectStore[Pointer Initializer] struct {
	ptr Pointer
}

type Initializer interface {
	Initialize()
}

type store interface {
	Initializer
	json.Marshaler
	json.Unmarshaler
}

var stores = make(map[namespace]store)
var storesPath = common.DataDir

func init() {
	task.OnProgramExit("save_stores", func() {
		if err := save(); err != nil {
			logging.Error().Err(err).Msg("failed to save stores")
		}
	})
}

func loadNS[T store](ns namespace) T {
	store := reflect.New(reflect.TypeFor[T]().Elem()).Interface().(T)
	store.Initialize()
	path := filepath.Join(storesPath, string(ns)+".json")
	file, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.Err(err).
				Str("path", path).
				Msg("failed to load store")
		}
	} else {
		defer file.Close()
		if err := json.NewDecoder(file).Decode(&store); err != nil {
			logging.Err(err).
				Str("path", path).
				Msg("failed to load store")
		}
	}
	stores[ns] = store
	logging.Debug().
		Str("namespace", string(ns)).
		Str("path", path).
		Msg("loaded store")
	return store
}

func save() error {
	errs := gperr.NewBuilder("failed to save data stores")
	for ns, store := range stores {
		if err := utils.SaveJSON(filepath.Join(storesPath, string(ns)+".json"), &store, 0o644); err != nil {
			errs.Add(err)
		}
	}
	return errs.Error()
}

func Store[VT any](namespace namespace) MapStore[VT] {
	if _, ok := stores[namespace]; ok {
		logging.Fatal().Str("namespace", string(namespace)).Msg("namespace already exists")
	}
	store := loadNS[*MapStore[VT]](namespace)
	stores[namespace] = store
	return *store
}

func Object[Ptr Initializer](namespace namespace) Ptr {
	if _, ok := stores[namespace]; ok {
		logging.Fatal().Str("namespace", string(namespace)).Msg("namespace already exists")
	}
	obj := loadNS[*ObjectStore[Ptr]](namespace)
	stores[namespace] = obj
	return obj.ptr
}

func (s *MapStore[VT]) Initialize() {
	s.Map = xsync.NewMap[string, VT]()
}

func (s MapStore[VT]) MarshalJSON() ([]byte, error) {
	return json.Marshal(maps.Collect(s.Range))
}

func (s *MapStore[VT]) UnmarshalJSON(data []byte) error {
	tmp := make(map[string]VT)
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	s.Map = xsync.NewMap[string, VT](xsync.WithPresize(len(tmp)))
	for k, v := range tmp {
		s.Map.Store(k, v)
	}
	return nil
}

func (obj *ObjectStore[Ptr]) Initialize() {
	obj.ptr = reflect.New(reflect.TypeFor[Ptr]().Elem()).Interface().(Ptr)
	obj.ptr.Initialize()
}

func (obj ObjectStore[Ptr]) MarshalJSON() ([]byte, error) {
	return json.Marshal(obj.ptr)
}

func (obj *ObjectStore[Ptr]) UnmarshalJSON(data []byte) error {
	obj.Initialize()
	return json.Unmarshal(data, obj.ptr)
}

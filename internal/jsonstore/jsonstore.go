package jsonstore

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"

	"maps"

	"github.com/puzpuzpuz/xsync/v3"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
)

type namespace string

type MapStore[VT any] struct {
	*xsync.MapOf[string, VT]
}

type ObjectStore[Pointer Initializer] struct {
	ptr Pointer
}

type Initializer interface {
	Initialize()
}

type storeByNamespace struct {
	sync.RWMutex
	m map[namespace]store
}

type store interface {
	json.Marshaler
	json.Unmarshaler
}

var stores = storeByNamespace{m: make(map[namespace]store)}
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
	for ns, obj := range stores.m {
		if init, ok := obj.(Initializer); ok {
			init.Initialize()
		}
		if err := utils.LoadJSONIfExist(filepath.Join(storesPath, string(ns)+".json"), &obj); err != nil {
			errs.Add(err)
		} else {
			logging.Info().Str("name", string(ns)).Msg("store loaded")
		}
		stores.m[ns] = obj
	}
	return errs.Error()
}

func save() error {
	stores.Lock()
	defer stores.Unlock()
	errs := gperr.NewBuilder("failed to save data stores")
	for ns, store := range stores.m {
		if err := utils.SaveJSON(filepath.Join(storesPath, string(ns)+".json"), &store, 0o644); err != nil {
			errs.Add(err)
		}
	}
	return errs.Error()
}

func Store[VT any](namespace namespace) MapStore[VT] {
	stores.Lock()
	defer stores.Unlock()
	if s, ok := stores.m[namespace]; ok {
		v, ok := s.(*MapStore[VT])
		if !ok {
			panic(fmt.Errorf("type mismatch: %T != %T", s, v))
		}
		return *v
	}
	m := &MapStore[VT]{MapOf: xsync.NewMapOf[string, VT]()}
	stores.m[namespace] = m
	return *m
}

func Object[Ptr Initializer](namespace namespace) Ptr {
	stores.Lock()
	defer stores.Unlock()
	if s, ok := stores.m[namespace]; ok {
		v, ok := s.(*ObjectStore[Ptr])
		if !ok {
			panic(fmt.Errorf("type mismatch: %T != %T", s, v))
		}
		return v.ptr
	}
	obj := &ObjectStore[Ptr]{}
	obj.init()
	stores.m[namespace] = obj
	return obj.ptr
}

func (s MapStore[VT]) MarshalJSON() ([]byte, error) {
	return json.Marshal(maps.Collect(s.Range))
}

func (s *MapStore[VT]) UnmarshalJSON(data []byte) error {
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

func (obj *ObjectStore[Ptr]) init() {
	obj.ptr = reflect.New(reflect.TypeFor[Ptr]().Elem()).Interface().(Ptr)
	obj.ptr.Initialize()
}

func (obj ObjectStore[Ptr]) MarshalJSON() ([]byte, error) {
	return json.Marshal(obj.ptr)
}

func (obj *ObjectStore[Ptr]) UnmarshalJSON(data []byte) error {
	obj.init()
	return json.Unmarshal(data, obj.ptr)
}

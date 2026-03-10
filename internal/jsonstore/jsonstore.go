package jsonstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/task"
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

var (
	stores     = make(map[namespace]store)
	storesPath = common.DataDir
)

func init() {
	task.OnProgramExit("save_stores", func() {
		if err := save(); err != nil {
			log.Error().Err(err).Msg("failed to save stores")
		}
	})
}

func loadNS[T store](ns namespace) T {
	store := reflect.New(reflect.TypeFor[T]().Elem()).Interface().(T)
	store.Initialize()

	if common.IsTest {
		return store
	}

	path := filepath.Join(storesPath, string(ns)+".json")
	file, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Err(err).
				Str("path", path).
				Msg("failed to load store")
		}
	} else {
		defer file.Close()
		if err := json.NewDecoder(file).Decode(&store); err != nil {
			log.Err(err).
				Str("path", path).
				Msg("failed to load store")
		}
	}
	stores[ns] = store
	log.Debug().
		Str("namespace", string(ns)).
		Str("path", path).
		Msg("loaded store")
	return store
}

func save() error {
	errs := gperr.NewBuilder("failed to save data stores")
	for ns, store := range stores {
		path := filepath.Join(storesPath, string(ns)+".json")
		if err := serialization.SaveFile(path, &store, 0o644, json.Marshal); err != nil {
			errs.Add(err)
		}
	}
	return errs.Error()
}

func Store[VT any](namespace namespace) MapStore[VT] {
	if _, ok := stores[namespace]; ok {
		log.Fatal().Str("namespace", string(namespace)).Msg("namespace already exists")
	}
	store := loadNS[*MapStore[VT]](namespace)
	stores[namespace] = store
	return *store
}

func Object[Ptr Initializer](namespace namespace) Ptr {
	if _, ok := stores[namespace]; ok {
		log.Fatal().Str("namespace", string(namespace)).Msg("namespace already exists")
	}
	obj := loadNS[*ObjectStore[Ptr]](namespace)
	stores[namespace] = obj
	return obj.ptr
}

func (s *MapStore[VT]) Initialize() {
	s.Map = xsync.NewMap[string, VT]()
}

func (s MapStore[VT]) MarshalJSON() ([]byte, error) {
	return json.Marshal(xsync.ToPlainMap(s.Map))
}

func (s *MapStore[VT]) UnmarshalJSON(data []byte) error {
	tmp := make(map[string]VT)
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	s.Map = xsync.NewMap[string, VT](xsync.WithPresize(len(tmp)))
	for k, v := range tmp {
		s.Store(k, v)
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

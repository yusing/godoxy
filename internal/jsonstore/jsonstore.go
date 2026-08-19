package jsonstore

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
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

// saveInterval bounds how much of a store is lost when the process is killed
// without running the shutdown callbacks (SIGKILL, OOM, host reset).
const saveInterval = 30 * time.Second

var (
	mu         sync.Mutex // guards stores and lastSaved
	stores     = make(map[namespace]store)
	lastSaved  = make(map[namespace][]byte)
	storesPath = common.DataDir
)

func init() {
	go saveEvery(task.RootContext(), saveInterval)
}

// Save writes every store to disk.
//
// The caller must invoke this on the shutdown path once task.WaitExit returns.
// Registering it as an OnProgramExit callback is not enough: those callbacks
// run in detached goroutines that WaitExit stops waiting for as soon as a child
// task exceeds the shutdown timeout, so the process can exit mid-write.
func Save() {
	if err := save(); err != nil {
		log.Error().Err(err).Msg("failed to save stores")
	}
}

func saveEvery(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := save(); err != nil {
				log.Error().Err(err).Msg("failed to save stores")
			}
		case <-ctx.Done():
			return
		}
	}
}

func loadNS[T store](ns namespace) T {
	store := reflect.New(reflect.TypeFor[T]().Elem()).Interface().(T)
	store.Initialize()

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
		if err := strutils.NewJSONDecoder(file).Decode(&store); err != nil {
			log.Err(err).
				Str("path", path).
				Msg("failed to load store")
		}
	}
	log.Debug().
		Str("namespace", string(ns)).
		Str("path", path).
		Msg("loaded store")
	return store
}

func save() error {
	mu.Lock()
	defer mu.Unlock()

	errs := gperr.NewBuilder("failed to save data stores")
	for ns, store := range stores {
		if err := saveNS(ns, store); err != nil {
			errs.Add(err)
		}
	}
	return errs.Error()
}

// saveNS writes the store atomically, and skips the write when its content is
// unchanged since the last successful save.
func saveNS(ns namespace, store store) error {
	data, err := strutils.MarshalJSON(store)
	if err != nil {
		return err
	}
	if bytes.Equal(lastSaved[ns], data) {
		return nil
	}

	path := filepath.Join(storesPath, string(ns)+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	lastSaved[ns] = data
	log.Debug().
		Str("namespace", string(ns)).
		Str("path", path).
		Msg("saved store")
	return nil
}

func Store[VT any](namespace namespace) MapStore[VT] {
	mu.Lock()
	defer mu.Unlock()

	if _, ok := stores[namespace]; ok {
		log.Fatal().Str("namespace", string(namespace)).Msg("namespace already exists")
	}
	store := loadNS[*MapStore[VT]](namespace)
	stores[namespace] = store
	return *store
}

func Object[Ptr Initializer](namespace namespace) Ptr {
	mu.Lock()
	defer mu.Unlock()

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
	return strutils.MarshalJSON(xsync.ToPlainMap(s.Map))
}

func (s *MapStore[VT]) UnmarshalJSON(data []byte) error {
	tmp := make(map[string]VT)
	if err := strutils.UnmarshalJSON(data, &tmp); err != nil {
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
	return strutils.MarshalJSON(obj.ptr)
}

func (obj *ObjectStore[Ptr]) UnmarshalJSON(data []byte) error {
	obj.Initialize()
	return strutils.UnmarshalJSON(data, obj.ptr)
}

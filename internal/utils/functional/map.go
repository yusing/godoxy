package functional

import (
	"sync"

	"github.com/puzpuzpuz/xsync/v3"
	"gopkg.in/yaml.v3"
)

type Map[KT comparable, VT any] struct {
	*xsync.MapOf[KT, VT]
}

const minParallelSize = 4

func NewMapOf[KT comparable, VT any](options ...func(*xsync.MapConfig)) Map[KT, VT] {
	return Map[KT, VT]{xsync.NewMapOf[KT, VT](options...)}
}

func NewMapFrom[KT comparable, VT any](m map[KT]VT) (res Map[KT, VT]) {
	res = NewMapOf[KT, VT](xsync.WithPresize(len(m)))
	for k, v := range m {
		res.Store(k, v)
	}
	return
}

func NewMap[MapType Map[KT, VT], KT comparable, VT any]() Map[KT, VT] {
	return NewMapOf[KT, VT]()
}

// RangeAll calls the given function for each key-value pair in the map.
//
// Parameters:
//
//	do: function to call for each key-value pair
//
// Returns:
//
//	nothing
func (m Map[KT, VT]) RangeAll(do func(k KT, v VT)) {
	m.Range(func(k KT, v VT) bool {
		do(k, v)
		return true
	})
}

// RangeAllParallel calls the given function for each key-value pair in the map,
// in parallel. The map is not safe for modification from within the function.
//
// Parameters:
//
//	do: function to call for each key-value pair
//
// Returns:
//
//	nothing
func (m Map[KT, VT]) RangeAllParallel(do func(k KT, v VT)) {
	if m.Size() < minParallelSize {
		m.RangeAll(do)
		return
	}

	var wg sync.WaitGroup

	m.Range(func(k KT, v VT) bool {
		wg.Add(1)
		go func() {
			do(k, v)
			wg.Done()
		}()
		return true
	})
	wg.Wait()
}

// CollectErrors calls the given function for each key-value pair in the map,
// then returns a slice of errors collected.
func (m Map[KT, VT]) CollectErrors(do func(k KT, v VT) error) []error {
	errs := make([]error, 0)
	m.Range(func(k KT, v VT) bool {
		if err := do(k, v); err != nil {
			errs = append(errs, err)
		}
		return true
	})
	return errs
}

// CollectErrors calls the given function for each key-value pair in the map,
// then returns a slice of errors collected.
func (m Map[KT, VT]) CollectErrorsParallel(do func(k KT, v VT) error) []error {
	if m.Size() < minParallelSize {
		return m.CollectErrors(do)
	}

	var errs []error
	var mu sync.Mutex
	var wg sync.WaitGroup

	m.Range(func(k KT, v VT) bool {
		wg.Add(1)
		go func() {
			if err := do(k, v); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
			wg.Done()
		}()
		return true
	})
	wg.Wait()
	return errs
}

func (m Map[KT, VT]) Has(k KT) bool {
	_, ok := m.Load(k)
	return ok
}

func (m Map[KT, VT]) String() string {
	tmp := make(map[KT]VT, m.Size())
	m.RangeAll(func(k KT, v VT) {
		tmp[k] = v
	})
	data, err := yaml.Marshal(tmp)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

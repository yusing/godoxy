package trie

import (
	"sync/atomic"
)

// AnyValue is a wrapper of atomic.Value
// It is used to store values in trie nodes
// And allowed to assign to empty struct value when node
// is not an end node anymore
type AnyValue struct {
	v atomic.Value
}

type zeroValue struct{}

var zero zeroValue

func (av *AnyValue) Store(v any) {
	if v == nil {
		av.v.Store(zero)
		return
	}
	defer panicInvalidAssignment()
	av.v.Store(v)
}

func (av *AnyValue) Swap(v any) any {
	defer panicInvalidAssignment()
	return av.v.Swap(v)
}

func (av *AnyValue) Load() any {
	switch v := av.v.Load().(type) {
	case zeroValue:
		return nil
	default:
		return v
	}
}

func (av *AnyValue) IsNil() bool {
	switch v := av.v.Load().(type) {
	case zeroValue:
		return true // assigned nil manually
	default:
		return v == nil // uninitialized
	}
}

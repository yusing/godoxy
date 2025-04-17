package functional_test

import (
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/functional"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestNewMapFrom(t *testing.T) {
	m := NewMapFrom(map[string]int{
		"a": 1,
		"b": 2,
		"c": 3,
	})
	expect.Equal(t, m.Size(), 3)
	expect.True(t, m.Has("a"))
	expect.True(t, m.Has("b"))
	expect.True(t, m.Has("c"))
}

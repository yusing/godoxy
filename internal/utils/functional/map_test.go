package functional_test

import (
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/functional"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestNewMapFrom(t *testing.T) {
	m := NewMapFrom(map[string]int{
		"a": 1,
		"b": 2,
		"c": 3,
	})
	ExpectEqual(t, m.Size(), 3)
	ExpectTrue(t, m.Has("a"))
	ExpectTrue(t, m.Has("b"))
	ExpectTrue(t, m.Has("c"))
}

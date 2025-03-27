package gperr_test

import (
	"context"
	"errors"
	"io"
	"testing"

	. "github.com/yusing/go-proxy/internal/gperr"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestBuilderEmpty(t *testing.T) {
	eb := NewBuilder("foo")
	ExpectTrue(t, errors.Is(eb.Error(), nil))
	ExpectFalse(t, eb.HasError())
}

func TestBuilderAddNil(t *testing.T) {
	eb := NewBuilder("foo")
	var err Error
	for range 3 {
		eb.Add(nil)
	}
	for range 3 {
		eb.Add(err)
	}
	eb.AddRange(nil, nil, err)
	ExpectFalse(t, eb.HasError())
	ExpectTrue(t, eb.Error() == nil)
}

func TestBuilderIs(t *testing.T) {
	eb := NewBuilder("foo")
	eb.Add(context.Canceled)
	eb.Add(io.ErrShortBuffer)
	ExpectTrue(t, eb.HasError())
	ExpectError(t, io.ErrShortBuffer, eb.Error())
	ExpectError(t, context.Canceled, eb.Error())
}

func TestBuilderNested(t *testing.T) {
	eb := NewBuilder("action failed")
	eb.Add(New("Action 1").Withf("Inner: 1").Withf("Inner: 2"))
	eb.Add(New("Action 2").Withf("Inner: 3"))

	got := eb.String()
	expected := `action failed
  • Action 1
    • Inner: 1
    • Inner: 2
  • Action 2
    • Inner: 3`
	ExpectEqual(t, got, expected)
}

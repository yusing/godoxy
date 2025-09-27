package gperr

import (
	"errors"
	"strings"
	"testing"

	expect "github.com/yusing/godoxy/internal/utils/testing"
	"github.com/yusing/goutils/strings/ansi"
)

func TestBaseString(t *testing.T) {
	expect.Equal(t, New("error").Error(), "error")
}

func TestBaseWithSubject(t *testing.T) {
	err := New("error")
	withSubject := err.Subject("foo")
	withSubjectf := err.Subjectf("%s %s", "foo", "bar")

	expect.ErrorIs(t, err, withSubject)
	expect.Equal(t, ansi.StripANSI(withSubject.Error()), "foo: error")
	expect.True(t, withSubject.Is(err))

	expect.ErrorIs(t, err, withSubjectf)
	expect.Equal(t, ansi.StripANSI(withSubjectf.Error()), "foo bar: error")
	expect.True(t, withSubjectf.Is(err))
}

func TestBaseWithExtra(t *testing.T) {
	err := New("error")
	extra := New("bar").Subject("baz")
	withExtra := err.With(extra)

	expect.True(t, withExtra.Is(extra))
	expect.True(t, withExtra.Is(err))

	expect.True(t, errors.Is(withExtra, extra))
	expect.True(t, errors.Is(withExtra, err))

	expect.True(t, strings.Contains(withExtra.Error(), err.Error()))
	expect.True(t, strings.Contains(withExtra.Error(), extra.Error()))
	expect.True(t, strings.Contains(withExtra.Error(), "baz"))
}

func TestBaseUnwrap(t *testing.T) {
	err := errors.New("err")
	wrapped := Wrap(err)

	expect.ErrorIs(t, err, errors.Unwrap(wrapped))
}

func TestNestedUnwrap(t *testing.T) {
	err := errors.New("err")
	err2 := New("err2")
	wrapped := Wrap(err).Subject("foo").With(err2.Subject("bar"))

	unwrapper, ok := wrapped.(interface{ Unwrap() []error })
	expect.True(t, ok)

	expect.ErrorIs(t, err, wrapped)
	expect.ErrorIs(t, err2, wrapped)
	expect.Equal(t, len(unwrapper.Unwrap()), 2)
}

func TestErrorIs(t *testing.T) {
	from := errors.New("error")
	err := Wrap(from)
	expect.ErrorIs(t, from, err)

	expect.True(t, err.Is(from))
	expect.False(t, err.Is(New("error")))

	expect.True(t, errors.Is(err.Subject("foo"), from))
	expect.True(t, errors.Is(err.Withf("foo"), from))
	expect.True(t, errors.Is(err.Subject("foo").Withf("bar"), from))
}

func TestErrorImmutability(t *testing.T) {
	err := New("err")
	err2 := New("err2")

	for range 3 {
		// t.Logf("%d: %v %T %s", i, errors.Unwrap(err), err, err)
		_ = err.Subject("foo")
		expect.False(t, strings.Contains(err.Error(), "foo"))

		_ = err.With(err2)
		expect.False(t, strings.Contains(err.Error(), "extra"))
		expect.False(t, err.Is(err2))

		err = err.Subject("bar").Withf("baz")
		expect.True(t, err != nil)
	}
}

func TestErrorWith(t *testing.T) {
	err1 := New("err1")
	err2 := New("err2")

	err3 := err1.With(err2)

	expect.True(t, err3.Is(err1))
	expect.True(t, err3.Is(err2))

	_ = err2.Subject("foo")

	expect.True(t, err3.Is(err1))
	expect.True(t, err3.Is(err2))

	// check if err3 is affected by err2.Subject
	expect.False(t, strings.Contains(err3.Error(), "foo"))
}

func TestErrorStringSimple(t *testing.T) {
	errFailure := New("generic failure")
	ne := errFailure.Subject("foo bar")
	expect.Equal(t, ansi.StripANSI(ne.Error()), "foo bar: generic failure")
	ne = ne.Subject("baz")
	expect.Equal(t, ansi.StripANSI(ne.Error()), "baz > foo bar: generic failure")
}

func TestErrorStringNested(t *testing.T) {
	errFailure := New("generic failure")
	inner := errFailure.Subject("inner").
		Withf("1").
		Withf("1")
	inner2 := errFailure.Subject("inner2").
		Subject("action 2").
		Withf("2").
		Withf("2")
	inner3 := errFailure.Subject("inner3").
		Subject("action 3").
		Withf("3").
		Withf("3")
	ne := errFailure.
		Subject("foo").
		Withf("bar").
		Withf("baz").
		With(inner).
		With(inner.With(inner2.With(inner3)))
	want := `foo: generic failure
  • bar
  • baz
  • inner: generic failure
    • 1
    • 1
  • inner: generic failure
    • 1
    • 1
    • action 2 > inner2: generic failure
      • 2
      • 2
      • action 3 > inner3: generic failure
        • 3
        • 3
`
	expect.Equal(t, ansi.StripANSI(ne.Error()), want)
}

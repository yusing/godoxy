package gperr

import (
	"errors"
	"strings"
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestBaseString(t *testing.T) {
	ExpectEqual(t, New("error").Error(), "error")
}

func TestBaseWithSubject(t *testing.T) {
	err := New("error")
	withSubject := err.Subject("foo")
	withSubjectf := err.Subjectf("%s %s", "foo", "bar")

	ExpectError(t, err, withSubject)
	ExpectEqual(t, withSubject.Error(), "foo: error")
	ExpectTrue(t, withSubject.Is(err))

	ExpectError(t, err, withSubjectf)
	ExpectEqual(t, withSubjectf.Error(), "foo bar: error")
	ExpectTrue(t, withSubjectf.Is(err))
}

func TestBaseWithExtra(t *testing.T) {
	err := New("error")
	extra := New("bar").Subject("baz")
	withExtra := err.With(extra)

	ExpectTrue(t, withExtra.Is(extra))
	ExpectTrue(t, withExtra.Is(err))

	ExpectTrue(t, errors.Is(withExtra, extra))
	ExpectTrue(t, errors.Is(withExtra, err))

	ExpectTrue(t, strings.Contains(withExtra.Error(), err.Error()))
	ExpectTrue(t, strings.Contains(withExtra.Error(), extra.Error()))
	ExpectTrue(t, strings.Contains(withExtra.Error(), "baz"))
}

func TestBaseUnwrap(t *testing.T) {
	err := errors.New("err")
	wrapped := Wrap(err)

	ExpectError(t, err, errors.Unwrap(wrapped))
}

func TestNestedUnwrap(t *testing.T) {
	err := errors.New("err")
	err2 := New("err2")
	wrapped := Wrap(err).Subject("foo").With(err2.Subject("bar"))

	unwrapper, ok := wrapped.(interface{ Unwrap() []error })
	ExpectTrue(t, ok)

	ExpectError(t, err, wrapped)
	ExpectError(t, err2, wrapped)
	ExpectEqual(t, len(unwrapper.Unwrap()), 2)
}

func TestErrorIs(t *testing.T) {
	from := errors.New("error")
	err := Wrap(from)
	ExpectError(t, from, err)

	ExpectTrue(t, err.Is(from))
	ExpectFalse(t, err.Is(New("error")))

	ExpectTrue(t, errors.Is(err.Subject("foo"), from))
	ExpectTrue(t, errors.Is(err.Withf("foo"), from))
	ExpectTrue(t, errors.Is(err.Subject("foo").Withf("bar"), from))
}

func TestErrorImmutability(t *testing.T) {
	err := New("err")
	err2 := New("err2")

	for range 3 {
		// t.Logf("%d: %v %T %s", i, errors.Unwrap(err), err, err)
		_ = err.Subject("foo")
		ExpectFalse(t, strings.Contains(err.Error(), "foo"))

		_ = err.With(err2)
		ExpectFalse(t, strings.Contains(err.Error(), "extra"))
		ExpectFalse(t, err.Is(err2))

		err = err.Subject("bar").Withf("baz")
		ExpectTrue(t, err != nil)
	}
}

func TestErrorWith(t *testing.T) {
	err1 := New("err1")
	err2 := New("err2")

	err3 := err1.With(err2)

	ExpectTrue(t, err3.Is(err1))
	ExpectTrue(t, err3.Is(err2))

	_ = err2.Subject("foo")

	ExpectTrue(t, err3.Is(err1))
	ExpectTrue(t, err3.Is(err2))

	// check if err3 is affected by err2.Subject
	ExpectFalse(t, strings.Contains(err3.Error(), "foo"))
}

func TestErrorStringSimple(t *testing.T) {
	errFailure := New("generic failure")
	ne := errFailure.Subject("foo bar")
	ExpectEqual(t, ne.Error(), "foo bar: generic failure")
	ne = ne.Subject("baz")
	ExpectEqual(t, ne.Error(), "baz > foo bar: generic failure")
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
        • 3`
	ExpectEqual(t, ne.Error(), want)
}

package utils

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/utils/strutils/ansi"
)

func init() {
	if common.IsTest {
		os.Args = append([]string{os.Args[0], "-test.v"}, os.Args[1:]...)
	}
}

func Must[Result any](r Result, err error) Result {
	if err != nil {
		panic(err)
	}
	return r
}

func fmtError(err error) string {
	if err == nil {
		return "<nil>"
	}
	return ansi.StripANSI(err.Error())
}

func ExpectNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("expected err=nil, got %s", fmtError(err))
		t.FailNow()
	}
}

func ExpectHasError(t *testing.T, err error) {
	t.Helper()
	if errors.Is(err, nil) {
		t.Error("expected err not nil")
		t.FailNow()
	}
}

func ExpectError(t *testing.T, expected error, err error) {
	t.Helper()
	if !errors.Is(err, expected) {
		t.Errorf("expected err %s, got %s", expected, fmtError(err))
		t.FailNow()
	}
}

func ExpectError2(t *testing.T, input any, expected error, err error) {
	t.Helper()
	if !errors.Is(err, expected) {
		t.Errorf("%v: expected err %s, got %s", input, expected, fmtError(err))
		t.FailNow()
	}
}

func ExpectErrorT[T error](t *testing.T, err error) {
	t.Helper()
	var errAs T
	if !errors.As(err, &errAs) {
		t.Errorf("expected err %T, got %s", errAs, fmtError(err))
		t.FailNow()
	}
}

func ExpectEqual[T comparable](t *testing.T, got T, want T) {
	t.Helper()
	if gotStr, ok := any(got).(string); ok {
		ExpectDeepEqual(t, ansi.StripANSI(gotStr), any(want).(string))
		return
	}
	if got != want {
		t.Errorf("expected:\n%v, got\n%v", want, got)
		t.FailNow()
	}
}

func ExpectEqualAny[T comparable](t *testing.T, got T, wants []T) {
	t.Helper()
	for _, want := range wants {
		if got == want {
			return
		}
	}
	t.Errorf("expected any of:\n%v, got\n%v", wants, got)
	t.FailNow()
}

func ExpectDeepEqual[T any](t *testing.T, got T, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected:\n%v, got\n%v", want, got)
		t.FailNow()
	}
}

func ExpectBytesEqual(t *testing.T, got []byte, want []byte) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Errorf("expected:\n%v, got\n%v", want, got)
		t.FailNow()
	}
}

func ExpectTrue(t *testing.T, got bool) {
	t.Helper()
	if !got {
		t.Error("expected true")
		t.FailNow()
	}
}

func ExpectFalse(t *testing.T, got bool) {
	t.Helper()
	if got {
		t.Error("expected false")
		t.FailNow()
	}
}

func ExpectType[T any](t *testing.T, got any) (_ T) {
	t.Helper()
	_, ok := got.(T)
	if !ok {
		t.Fatalf("expected type %s, got %T", reflect.TypeFor[T](), got)
	}
	return got.(T)
}

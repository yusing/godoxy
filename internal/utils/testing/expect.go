package expect

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var isTest = strings.HasSuffix(os.Args[0], ".test")

func init() {
	if isTest {
		// force verbose output
		os.Args = append([]string{os.Args[0], "-test.v"}, os.Args[1:]...)
	}
}

func Must[Result any](r Result, err error) Result {
	if err != nil {
		panic(err)
	}
	return r
}

var (
	NoError        = require.NoError
	HasError       = require.Error
	True           = require.True
	False          = require.False
	Nil            = require.Nil
	NotNil         = require.NotNil
	ErrorContains  = require.ErrorContains
	Panics         = require.Panics
	Greater        = require.Greater
	Less           = require.Less
	GreaterOrEqual = require.GreaterOrEqual
	LessOrEqual    = require.LessOrEqual
)

func ErrorIs(t *testing.T, expected error, err error, msgAndArgs ...any) {
	t.Helper()
	require.ErrorIs(t, err, expected, msgAndArgs...)
}

func ErrorT[T error](t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	var errAs T
	require.ErrorAs(t, err, &errAs, msgAndArgs...)
}

func Equal[T any](t *testing.T, got T, want T, msgAndArgs ...any) {
	t.Helper()
	require.EqualValues(t, want, got, msgAndArgs...)
}

func NotEqual[T any](t *testing.T, got T, want T, msgAndArgs ...any) {
	t.Helper()
	require.NotEqual(t, want, got, msgAndArgs...)
}

func Contains[T any](t *testing.T, got T, wants []T, msgAndArgs ...any) {
	t.Helper()
	require.Contains(t, wants, got, msgAndArgs...)
}

func StringsContain(t *testing.T, got string, want string, msgAndArgs ...any) {
	t.Helper()
	require.Contains(t, got, want, msgAndArgs...)
}

func Type[T any](t *testing.T, got any, msgAndArgs ...any) (_ T) {
	t.Helper()
	_, ok := got.(T)
	require.True(t, ok, msgAndArgs...)
	return got.(T)
}

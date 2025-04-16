package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/go-proxy/internal/common"
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

func ExpectNoError(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	require.NoError(t, err, msgAndArgs...)
}

func ExpectHasError(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	require.Error(t, err, msgAndArgs...)
}

func ExpectError(t *testing.T, expected error, err error, msgAndArgs ...any) {
	t.Helper()
	require.ErrorIs(t, err, expected, msgAndArgs...)
}

func ExpectErrorT[T error](t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	var errAs T
	require.ErrorAs(t, err, &errAs, msgAndArgs...)
}

func ExpectEqual[T any](t *testing.T, got T, want T, msgAndArgs ...any) {
	t.Helper()
	require.Equal(t, want, got, msgAndArgs...)
}

func ExpectEqualValues(t *testing.T, got any, want any, msgAndArgs ...any) {
	t.Helper()
	require.EqualValues(t, want, got, msgAndArgs...)
}

func ExpectContains[T any](t *testing.T, got T, wants []T, msgAndArgs ...any) {
	t.Helper()
	require.Contains(t, wants, got, msgAndArgs...)
}

func ExpectTrue(t *testing.T, got bool, msgAndArgs ...any) {
	t.Helper()
	require.True(t, got, msgAndArgs...)
}

func ExpectFalse(t *testing.T, got bool, msgAndArgs ...any) {
	t.Helper()
	require.False(t, got, msgAndArgs...)
}

func ExpectType[T any](t *testing.T, got any, msgAndArgs ...any) (_ T) {
	t.Helper()
	_, ok := got.(T)
	require.True(t, ok, msgAndArgs...)
	return got.(T)
}

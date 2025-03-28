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

func ExpectNoError(t *testing.T, err error) {
	t.Helper()
	require.NoError(t, err)
}

func ExpectHasError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
}

func ExpectError(t *testing.T, expected error, err error) {
	t.Helper()
	require.ErrorIs(t, err, expected)
}

func ExpectErrorT[T error](t *testing.T, err error) {
	t.Helper()
	var errAs T
	require.ErrorAs(t, err, &errAs)
}

func ExpectEqual[T any](t *testing.T, got T, want T) {
	t.Helper()
	require.EqualValues(t, got, want)
}

func ExpectContains[T any](t *testing.T, got T, wants []T) {
	t.Helper()
	require.Contains(t, wants, got)
}

func ExpectTrue(t *testing.T, got bool) {
	t.Helper()
	require.True(t, got)
}

func ExpectFalse(t *testing.T, got bool) {
	t.Helper()
	require.False(t, got)
}

func ExpectType[T any](t *testing.T, got any) (_ T) {
	t.Helper()
	_, ok := got.(T)
	require.True(t, ok)
	return got.(T)
}

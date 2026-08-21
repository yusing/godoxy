package gphttp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNonUserRequestContext(t *testing.T) {
	ctx := t.Context()
	require.False(t, IsNonUserRequest(ctx))

	ctx = WithNonUserRequest(ctx)
	require.True(t, IsNonUserRequest(ctx))
}

package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/cenkalti/backoff/v6"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/agentpool"
)

func TestGetAgentSystemInfoWithRetryPreservesCancellationCause(t *testing.T) {
	pool := agentpool.NewPool()
	cfg := &agent.AgentConfig{Addr: "127.0.0.1:1"}
	require.True(t, pool.Add(cfg))
	a, ok := pool.Get(cfg.Addr)
	require.True(t, ok)

	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("metrics subscriber disconnected")
	cancel(cause)

	data, err := getAgentSystemInfoWithRetry(ctx, a, "")
	require.ErrorIs(t, err, cause)
	retryErr := backoff.AsRetryError(err)
	require.NotNil(t, retryErr)
	require.ErrorIs(t, retryErr.LastErr, cause)
	require.Nil(t, data.RawMessage)
	require.Nil(t, data.release)
}

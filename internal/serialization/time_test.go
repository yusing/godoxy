package serialization

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

// NOTE: -ldflags=-checklinkname=0 is required to test this function
func TestParseDuration(t *testing.T) {
	require.Equal(t, 24*time.Hour, expect.Must(time.ParseDuration("1d")))
	require.Equal(t, 7*24*time.Hour, expect.Must(time.ParseDuration("1w")))
	require.Equal(t, 30*24*time.Hour, expect.Must(time.ParseDuration("1M")))
}

package idlewatcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	idlewatchertypes "github.com/yusing/godoxy/internal/idlewatcher/runtime"
)

func TestSleepIn(t *testing.T) {
	w := &Watcher{
		cfg: &Config{
			IdlewatcherConfigBase: idlewatchertypes.ConfigBase{
				IdleTimeout: time.Minute,
			},
		},
	}
	w.state.Store(&containerState{
		status: idlewatchertypes.ContainerStatusRunning,
		ready:  true,
	})
	w.lastReset.Store(time.Now().Add(-30 * time.Second))

	remaining := w.SleepIn()
	require.Greater(t, remaining, 29*time.Second)
	require.LessOrEqual(t, remaining, 30*time.Second)
}

func TestSleepInHiddenWhenNotReadyOrExpired(t *testing.T) {
	w := &Watcher{
		cfg: &Config{
			IdlewatcherConfigBase: idlewatchertypes.ConfigBase{
				IdleTimeout: time.Minute,
			},
		},
	}
	w.state.Store(&containerState{
		status: idlewatchertypes.ContainerStatusStopped,
		ready:  false,
	})
	w.lastReset.Store(time.Now())
	require.Zero(t, w.SleepIn())

	w.state.Store(&containerState{
		status: idlewatchertypes.ContainerStatusRunning,
		ready:  true,
	})
	w.lastReset.Store(time.Now().Add(-2 * time.Minute))
	require.Zero(t, w.SleepIn())
}

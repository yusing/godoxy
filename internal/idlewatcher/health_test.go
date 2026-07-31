package idlewatcher

import (
	"errors"
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

func TestDetailReflectsCurrentState(t *testing.T) {
	testErr := errors.New("readiness timed out")
	tests := []struct {
		name  string
		state *containerState
		want  string
	}{
		{
			name: "healthy",
			state: &containerState{
				status: idlewatchertypes.ContainerStatusRunning,
				ready:  true,
			},
			want: "healthy",
		},
		{
			name: "starting",
			state: &containerState{
				status: idlewatchertypes.ContainerStatusRunning,
			},
			want: "starting",
		},
		{
			name: "napping",
			state: &containerState{
				status: idlewatchertypes.ContainerStatusStopped,
			},
			want: "napping",
		},
		{
			name: "error",
			state: &containerState{
				status: idlewatchertypes.ContainerStatusRunning,
				err:    testErr,
			},
			want: testErr.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Watcher{}
			w.state.Store(tt.state)

			require.Equal(t, tt.want, w.Detail())
		})
	}
}

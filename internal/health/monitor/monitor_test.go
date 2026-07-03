package monitor

import (
	"errors"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/goutils/task"
)

// Test notification tracker
type testNotificationTracker struct {
	mu                sync.RWMutex
	upNotifications   int
	downNotifications int
	lastNotification  string
}

func (t *testNotificationTracker) getStats() (up, down int, last string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.upNotifications, t.downNotifications, t.lastNotification
}

// Create test monitor with mock health checker - returns both monitor and tracker
func createTestMonitor(config health.HealthCheckConfig, checkFunc HealthCheckFunc) (*monitor, *testNotificationTracker) {
	testURL, _ := url.Parse("http://localhost:8080")

	var mon monitor
	mon.init(testURL, config, checkFunc)

	// Override notification functions to track calls instead of actually notifying
	tracker := &testNotificationTracker{}

	mon.notifyFunc = func(msg *notif.LogMessage) {
		tracker.mu.Lock()
		defer tracker.mu.Unlock()

		switch msg.Level {
		case zerolog.InfoLevel:
			tracker.upNotifications++
			tracker.lastNotification = "up"
		case zerolog.WarnLevel:
			tracker.downNotifications++
			tracker.lastNotification = "down"
		default:
			panic("unexpected log level: " + msg.Level.String())
		}
	}

	return &mon, tracker
}

func TestNotification_ImmediateNotifyAfterZero(t *testing.T) {
	config := health.HealthCheckConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  -1, // Immediate notification
	}

	mon, tracker := createTestMonitor(config, func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: true}, nil
	})

	// Start with healthy service
	result, err := mon.checkHealth(nil)
	require.NoError(t, err)
	require.True(t, result.Healthy)

	// Set to unhealthy
	mon.checkHealth = func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: false}, nil
	}

	// Simulate status change detection
	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// With NotifyAfter=0, notification should happen immediately
	require.Equal(t, health.StatusUnhealthy, mon.Status())

	// Check notification counts - should have 1 down notification
	up, down, last := tracker.getStats()
	require.Equal(t, 1, down)
	require.Equal(t, 0, up)
	require.Equal(t, "down", last)
}

func TestNotification_WithNotifyAfterThreshold(t *testing.T) {
	config := health.HealthCheckConfig{
		Interval: 50 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  2, // Notify after 2 consecutive failures
	}

	mon, tracker := createTestMonitor(config, func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: true}, nil
	})

	// Start healthy
	mon.status.Store(health.StatusHealthy)

	// Set to unhealthy
	mon.checkHealth = func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: false}, nil
	}

	// First failure - should not notify yet
	err := mon.checkUpdateHealth()
	require.NoError(t, err)

	// Should have no notifications yet (threshold not met)
	up, down, _ := tracker.getStats()
	require.Equal(t, 0, down)
	require.Equal(t, 0, up)

	// Second failure - should trigger notification
	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// Now should have 1 down notification after threshold met
	up, down, last := tracker.getStats()
	require.Equal(t, 1, down)
	require.Equal(t, 0, up)
	require.Equal(t, "down", last)
}

func TestNotification_ServiceRecoversBeforeThreshold(t *testing.T) {
	config := health.HealthCheckConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  3, // Notify after 3 consecutive failures
	}

	mon, tracker := createTestMonitor(config, func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: true}, nil
	})

	// Start healthy
	mon.status.Store(health.StatusHealthy)

	// Set to unhealthy
	mon.checkHealth = func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: false}, nil
	}

	// First failure
	err := mon.checkUpdateHealth()
	require.NoError(t, err)

	// Second failure
	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// Should have no notifications yet
	up, down, _ := tracker.getStats()
	require.Equal(t, 0, down)
	require.Equal(t, 0, up)

	// Service recovers before third failure
	mon.checkHealth = func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: true}, nil
	}

	// Health check with recovery
	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// Should have no notifications because threshold was never met.
	// Recovery notification is only sent after a down notification was sent.
	up, down, last := tracker.getStats()
	require.Equal(t, 0, down)
	require.Equal(t, 0, up)
	require.Empty(t, last)
}

func TestNotification_ConsecutiveFailureReset(t *testing.T) {
	config := health.HealthCheckConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  2, // Notify after 2 consecutive failures
	}

	mon, tracker := createTestMonitor(config, func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: true}, nil
	})

	// Start healthy
	mon.status.Store(health.StatusHealthy)

	// Set to unhealthy
	mon.checkHealth = func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: false}, nil
	}

	// First failure
	err := mon.checkUpdateHealth()
	require.NoError(t, err)

	// Recover briefly
	mon.checkHealth = func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: true}, nil
	}

	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// Should have no notifications, consecutive failures should reset.
	// Recovery notification is only sent after a down notification was sent.
	up, down, _ := tracker.getStats()
	require.Equal(t, 0, down)
	require.Equal(t, 0, up)

	// Go down again - consecutive counter should start from 0
	mon.checkHealth = func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: false}, nil
	}

	// First failure after recovery
	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// Should still have no down notifications (need 2 consecutive)
	up, down, _ = tracker.getStats()
	require.Equal(t, 0, down)
	require.Equal(t, 0, up)

	// Second consecutive failure - should trigger notification
	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// Now should have down notification
	up, down, last := tracker.getStats()
	require.Equal(t, 1, down)
	require.Equal(t, 0, up)
	require.Equal(t, "down", last)
}

func TestNotification_ContextCancellation(t *testing.T) {
	config := health.HealthCheckConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  1,
	}

	mon, tracker := createTestMonitor(config, func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: true}, nil
	})

	// Create a task that we can cancel
	rootTask := task.RootTask("test", true)
	mon.task = rootTask.Subtask("monitor", true)

	// Start healthy, then go unhealthy
	mon.status.Store(health.StatusHealthy)
	mon.checkHealth = func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: false}, nil
	}

	// Trigger notification
	err := mon.checkUpdateHealth()
	require.NoError(t, err)

	// Should have down notification
	up, down, _ := tracker.getStats()
	require.Equal(t, 1, down)
	require.Equal(t, 0, up)

	// Cancel the task context
	rootTask.Finish(nil)

	// Context cancellation doesn't affect notifications that already happened
	up, down, _ = tracker.getStats()
	require.Equal(t, 1, down)
	require.Equal(t, 0, up)
}

func TestCheckHealthAgentProxiedReturnsUnhealthyForInvalidURL(t *testing.T) {
	tests := []struct {
		name   string
		url    *url.URL
		detail string
	}{
		{name: "nil", url: nil, detail: "no url specified"},
		{name: "no host", url: &url.URL{Scheme: "http"}, detail: "no host specified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CheckHealthAgentProxied(nil, time.Hour, tt.url)
			require.NoError(t, err)
			require.False(t, result.Healthy)
			require.Equal(t, tt.detail, result.Detail)
		})
	}
}

func TestImmediateUpNotificationAfterDownNotification(t *testing.T) {
	config := health.HealthCheckConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  2,
	}

	mon, tracker := createTestMonitor(config, func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: false}, nil
	})

	// Start unhealthy
	mon.status.Store(health.StatusUnhealthy)
	mon.downNotificationSent.Store(true)

	// Set to healthy
	mon.checkHealth = func(u *url.URL) (health.HealthCheckResult, error) {
		return health.HealthCheckResult{Healthy: true, Latency: 50 * time.Millisecond}, nil
	}

	// Trigger health check
	err := mon.checkUpdateHealth()
	require.NoError(t, err)

	// Up notification should happen immediately once a prior down notification exists.
	require.Equal(t, health.StatusHealthy, mon.Status())

	// Should have exactly 1 up notification immediately
	up, down, last := tracker.getStats()
	require.Equal(t, 1, up)
	require.Equal(t, 0, down)
	require.Equal(t, "up", last)
}

func TestMonitorStartCancelsDuringJitterDelay(t *testing.T) {
	prevJitter := healthMonitorJitterDelay
	healthMonitorJitterDelay = func() time.Duration {
		return 10 * time.Second
	}
	t.Cleanup(func() {
		healthMonitorJitterDelay = prevJitter
	})

	var checks atomic.Int32
	mon, _ := createTestMonitor(health.HealthCheckConfig{
		Interval: 50 * time.Millisecond,
		Timeout:  10 * time.Millisecond,
	}, func(u *url.URL) (health.HealthCheckResult, error) {
		checks.Add(1)
		return health.HealthCheckResult{Healthy: true}, nil
	})

	rootTask := task.RootTask("test", true)
	defer rootTask.FinishAndWait("done")

	require.NoError(t, mon.Start(rootTask))
	require.Eventually(t, func() bool {
		return checks.Load() == 1
	}, time.Second, 10*time.Millisecond)

	start := time.Now()
	mon.Finish(errors.New("reload"))
	require.Eventually(t, func() bool {
		return mon.Task().FinishCause() != nil
	}, time.Second, 10*time.Millisecond)
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, int32(1), checks.Load(), "monitor should exit before post-check ticker starts")
}

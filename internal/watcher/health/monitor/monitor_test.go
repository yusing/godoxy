package monitor

import (
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/yusing/go-proxy/internal/notif"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher/health"
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
func createTestMonitor(config *health.HealthCheckConfig, checkFunc HealthCheckFunc) (*monitor, *testNotificationTracker) {
	testURL, _ := url.Parse("http://localhost:8080")

	mon := newMonitor(testURL, config, checkFunc)

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

	return mon, tracker
}

func TestNotification_ImmediateNotifyAfterZero(t *testing.T) {
	config := &health.HealthCheckConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  -1, // Immediate notification
	}

	mon, tracker := createTestMonitor(config, func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: true}, nil
	})

	// Start with healthy service
	result, err := mon.checkHealth()
	require.NoError(t, err)
	require.True(t, result.Healthy)

	// Set to unhealthy
	mon.checkHealth = func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: false}, nil
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
	config := &health.HealthCheckConfig{
		Interval: 50 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  2, // Notify after 2 consecutive failures
	}

	mon, tracker := createTestMonitor(config, func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: true}, nil
	})

	// Start healthy
	mon.status.Store(health.StatusHealthy)

	// Set to unhealthy
	mon.checkHealth = func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: false}, nil
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
	config := &health.HealthCheckConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  3, // Notify after 3 consecutive failures
	}

	mon, tracker := createTestMonitor(config, func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: true}, nil
	})

	// Start healthy
	mon.status.Store(health.StatusHealthy)

	// Set to unhealthy
	mon.checkHealth = func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: false}, nil
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
	mon.checkHealth = func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: true}, nil
	}

	// Health check with recovery
	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// Should have 1 up notification, but no down notification
	// because threshold was never met
	up, down, last := tracker.getStats()
	require.Equal(t, 0, down)
	require.Equal(t, 1, up)
	require.Equal(t, "up", last)
}

func TestNotification_ConsecutiveFailureReset(t *testing.T) {
	config := &health.HealthCheckConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  2, // Notify after 2 consecutive failures
	}

	mon, tracker := createTestMonitor(config, func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: true}, nil
	})

	// Start healthy
	mon.status.Store(health.StatusHealthy)

	// Set to unhealthy
	mon.checkHealth = func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: false}, nil
	}

	// First failure
	err := mon.checkUpdateHealth()
	require.NoError(t, err)

	// Recover briefly
	mon.checkHealth = func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: true}, nil
	}

	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// Should have 1 up notification, consecutive failures should reset
	up, down, _ := tracker.getStats()
	require.Equal(t, 0, down)
	require.Equal(t, 1, up)

	// Go down again - consecutive counter should start from 0
	mon.checkHealth = func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: false}, nil
	}

	// First failure after recovery
	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// Should still have no down notifications (need 2 consecutive)
	up, down, _ = tracker.getStats()
	require.Equal(t, 0, down)
	require.Equal(t, 1, up)

	// Second consecutive failure - should trigger notification
	err = mon.checkUpdateHealth()
	require.NoError(t, err)

	// Now should have down notification
	up, down, last := tracker.getStats()
	require.Equal(t, 1, down)
	require.Equal(t, 1, up)
	require.Equal(t, "down", last)
}

func TestNotification_ContextCancellation(t *testing.T) {
	config := &health.HealthCheckConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  1,
	}

	mon, tracker := createTestMonitor(config, func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: true}, nil
	})

	// Create a task that we can cancel
	rootTask := task.RootTask("test", true)
	mon.task = rootTask.Subtask("monitor", true)

	// Start healthy, then go unhealthy
	mon.status.Store(health.StatusHealthy)
	mon.checkHealth = func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: false}, nil
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

func TestImmediateUpNotification(t *testing.T) {
	config := &health.HealthCheckConfig{
		Interval: 100 * time.Millisecond,
		Timeout:  50 * time.Millisecond,
		Retries:  2, // NotifyAfter should not affect up notifications
	}

	mon, tracker := createTestMonitor(config, func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: false}, nil
	})

	// Start unhealthy
	mon.status.Store(health.StatusUnhealthy)

	// Set to healthy
	mon.checkHealth = func() (*health.HealthCheckResult, error) {
		return &health.HealthCheckResult{Healthy: true, Latency: 50 * time.Millisecond}, nil
	}

	// Trigger health check
	err := mon.checkUpdateHealth()
	require.NoError(t, err)

	// Up notification should happen immediately regardless of NotifyAfter setting
	require.Equal(t, health.StatusHealthy, mon.Status())

	// Should have exactly 1 up notification immediately
	up, down, last := tracker.getStats()
	require.Equal(t, 1, up)
	require.Equal(t, 0, down)
	require.Equal(t, "up", last)
}

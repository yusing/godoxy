package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	goproxmox "github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
)

const testTaskID = "UPID:pve:00000000:00000000:00000000:vzaction:101:root@pam:"

type lxcActionServerOptions struct {
	upid            string
	taskStatus      string
	exitStatus      string
	containerStatus LXCStatus
	taskPolled      chan<- struct{}
}

func newLXCActionTestNode(t *testing.T, opts lxcActionServerOptions) (*Node, *atomic.Int32) {
	t.Helper()
	actionCalls := new(atomic.Int32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/nodes/pve/lxc/101/status/"):
			actionCalls.Add(1)
			_, _ = fmt.Fprintf(w, `{"data":%q}`, opts.upid)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/nodes/pve/tasks/"):
			if opts.taskPolled != nil {
				select {
				case opts.taskPolled <- struct{}{}:
				default:
				}
			}
			_, _ = fmt.Fprintf(w, `{"data":{"status":%q,"exitstatus":%q}}`, opts.taskStatus, opts.exitStatus)
		case r.Method == http.MethodGet && r.URL.Path == "/nodes/pve/lxc/101/status/current":
			_, _ = fmt.Fprintf(w, `{"data":{"status":%q}}`, opts.containerStatus)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, goproxmox.WithHTTPClient(srv.Client()))
	return NewNode(client, "pve", "node/pve"), actionCalls
}

func TestLXCActionCompletesForEverySupportedAction(t *testing.T) {
	tests := []struct {
		name   string
		action LXCAction
		status LXCStatus
	}{
		{name: "start", action: LXCStart, status: LXCStatusRunning},
		{name: "shutdown", action: LXCShutdown, status: LXCStatusStopped},
		{name: "suspend", action: LXCSuspend, status: LXCStatusSuspended},
		{name: "resume", action: LXCResume, status: LXCStatusRunning},
		{name: "reboot", action: LXCReboot, status: LXCStatusRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, actionCalls := newLXCActionTestNode(t, lxcActionServerOptions{
				upid:            testTaskID,
				taskStatus:      "stopped",
				exitStatus:      "OK",
				containerStatus: tt.status,
			})

			require.NoError(t, node.LXCAction(t.Context(), 101, tt.action))
			require.EqualValues(t, 1, actionCalls.Load())
		})
	}
}

func TestLXCActionReturnsTaskFailure(t *testing.T) {
	node, _ := newLXCActionTestNode(t, lxcActionServerOptions{
		upid:       testTaskID,
		taskStatus: "stopped",
		exitStatus: "permission denied",
	})

	err := node.LXCAction(t.Context(), 101, LXCReboot)
	require.ErrorContains(t, err, "permission denied")
}

func TestLXCActionRejectsMalformedTaskID(t *testing.T) {
	node, _ := newLXCActionTestNode(t, lxcActionServerOptions{upid: "malformed"})

	err := node.LXCAction(t.Context(), 101, LXCStart)
	require.ErrorContains(t, err, "invalid task id")
}

func TestLXCActionRejectsUnknownActionBeforeRequest(t *testing.T) {
	node, actionCalls := newLXCActionTestNode(t, lxcActionServerOptions{})

	err := node.LXCAction(t.Context(), 101, LXCAction("future-action"))
	require.ErrorIs(t, err, ErrUnsupportedLXCAction)
	require.Zero(t, actionCalls.Load())
}

func TestLXCActionHonorsCancellationWhileWaiting(t *testing.T) {
	taskPolled := make(chan struct{}, 1)
	node, _ := newLXCActionTestNode(t, lxcActionServerOptions{
		upid:       testTaskID,
		taskStatus: "running",
		taskPolled: taskPolled,
	})
	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		errCh <- node.LXCAction(ctx, 101, LXCReboot)
	}()

	<-taskPolled
	cancel()
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func TestLXCActionHonorsCallerDeadlineWhenTaskDoesNotConverge(t *testing.T) {
	tests := []struct {
		name            string
		taskStatus      string
		exitStatus      string
		containerStatus LXCStatus
	}{
		{name: "task keeps running", taskStatus: "running"},
		{name: "container never reaches expected status", taskStatus: "stopped", exitStatus: "OK", containerStatus: LXCStatusStopped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskPolled := make(chan struct{}, 1)
			node, _ := newLXCActionTestNode(t, lxcActionServerOptions{
				upid:            testTaskID,
				taskStatus:      tt.taskStatus,
				exitStatus:      tt.exitStatus,
				containerStatus: tt.containerStatus,
				taskPolled:      taskPolled,
			})
			ctx, cancel := context.WithTimeout(t.Context(), 450*time.Millisecond)
			defer cancel()

			err := node.LXCAction(ctx, 101, LXCStart)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.NotEmpty(t, taskPolled)
		})
	}
}

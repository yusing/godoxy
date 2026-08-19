package jsonstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/yusing/goutils/task"
)

// TestShutdownFlushesStores runs the real signal -> task.WaitExit -> Save path
// in a subprocess, because WaitExit finishes the root task for the whole
// process.
//
// stuck mimics a subsystem that outlives the shutdown timeout: task.WaitExit
// stops waiting for OnProgramExit callbacks once a child task times out, so a
// store that is only written from a callback can be cut short by process exit.
func TestShutdownFlushesStores(t *testing.T) {
	for _, stuck := range []int{0, 1} {
		t.Run("stuck_task="+strconv.Itoa(stuck), func(t *testing.T) {
			dir := t.TempDir()

			cmd := exec.Command(os.Args[0], "-test.run=TestShutdownFlushesStores")
			cmd.Env = append(os.Environ(),
				"JSONSTORE_SHUTDOWN_CHILD=1",
				"JSONSTORE_SHUTDOWN_DIR="+dir,
				"JSONSTORE_SHUTDOWN_STUCK="+strconv.Itoa(stuck),
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("child failed: %v\n%s", err, out)
			}

			data, err := os.ReadFile(filepath.Join(dir, "shutdown.json"))
			if err != nil {
				t.Fatalf("store not written on shutdown: %v\nchild output:\n%s", err, out)
			}
			if string(data) != `{"a":"1"}` {
				t.Fatalf("unexpected store content %q", data)
			}
		})
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("JSONSTORE_SHUTDOWN_CHILD") != "1" {
		os.Exit(m.Run())
	}

	storesPath = os.Getenv("JSONSTORE_SHUTDOWN_DIR")
	store := Store[string]("shutdown")

	if os.Getenv("JSONSTORE_SHUTDOWN_STUCK") == "1" {
		stuck := task.RootTask("stuck", true)
		go func() {
			<-time.After(time.Minute)
			stuck.Finish(nil)
		}()
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		// the change lands after the ticker would have fired, so only the
		// shutdown write can persist it
		store.Store("a", "1")
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}()

	// mirrors cmd/main.go
	task.WaitExit(1)
	Save()
	os.Exit(0)
}

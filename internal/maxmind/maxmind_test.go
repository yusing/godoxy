package maxmind

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-golang"
	"github.com/yusing/godoxy/internal/common"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/goutils/task"
)

func testCfg() *MaxMind {
	return &MaxMind{
		Config: &Config{
			AccountID:  "testid",
			LicenseKey: "testkey",
			Database:   maxmind.MaxMindGeoLite,
		},
	}
}

var testLastMod = time.Date(2026, time.July, 9, 12, 34, 56, 0, time.UTC)

func testDoReq(cfg *MaxMind, w http.ResponseWriter, r *http.Request) {
	if u, p, ok := r.BasicAuth(); !ok || u != "testid" || p != "testkey" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Last-Modified", testLastMod.Format(http.TimeFormat))
	gz := gzip.NewWriter(w)
	t := tar.NewWriter(gz)
	_ = t.WriteHeader(&tar.Header{
		Name: cfg.dbFilename(),
	})
	_, _ = t.Write([]byte("1234"))
	_ = t.Close()
	_ = gz.Close()
	w.WriteHeader(http.StatusOK)
}

func mockDoReq(t *testing.T, cfg *MaxMind) {
	t.Helper()
	rw := httptest.NewRecorder()
	oldDoReq := doReq
	doReq = func(req *http.Request) (*http.Response, error) {
		testDoReq(cfg, rw, req)
		return rw.Result(), nil
	}
	t.Cleanup(func() { doReq = oldDoReq })
}

func mockDataDir(t *testing.T) {
	t.Helper()
	oldDataDir := dataDir
	dataDir = t.TempDir()
	t.Cleanup(func() { dataDir = oldDataDir })
}

func mockMaxMindDBOpen(t *testing.T) {
	t.Helper()
	oldMaxMindDBOpen := maxmindDBOpen
	maxmindDBOpen = func(path string) (*maxminddb.Reader, error) {
		return &maxminddb.Reader{}, nil
	}
	t.Cleanup(func() { maxmindDBOpen = oldMaxMindDBOpen })
}

func Test_MaxMindConfig_doReq(t *testing.T) {
	cfg := testCfg()
	mockDoReq(t, cfg)
	resp, err := cfg.doReq(t.Context(), http.MethodGet)
	if err != nil {
		t.Fatalf("newReq() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected status: %v", resp.StatusCode)
	}
}

func Test_MaxMindConfig_checkLatest(t *testing.T) {
	cfg := testCfg()
	mockDoReq(t, cfg)

	latest, err := cfg.checkLastest(t.Context())
	if err != nil {
		t.Fatalf("checkLatest() error = %v", err)
	}
	if !latest.Equal(testLastMod) {
		t.Errorf("latest = %v, want %v", latest, testLastMod)
	}
}

func Test_MaxMindConfig_download(t *testing.T) {
	cfg := testCfg()
	mockDataDir(t)
	mockMaxMindDBOpen(t)
	mockDoReq(t, cfg)

	err := cfg.download(t.Context())
	if err != nil {
		t.Fatalf("download() error = %v", err)
	}
	if cfg.db.Reader == nil {
		t.Error("expected db instance")
	}
}

func Test_MaxMindConfig_loadMaxMindDBSchedulesUpdateAfterDownload(t *testing.T) {
	cfg := testCfg()
	mockDataDir(t)
	mockDoReq(t, cfg)

	oldIsTest := common.IsTest
	common.IsTest = false
	t.Cleanup(func() { common.IsTest = oldIsTest })

	oldMaxMindDBOpen := maxmindDBOpen
	dbMissing := true
	maxmindDBOpen = func(path string) (*maxminddb.Reader, error) {
		if dbMissing {
			dbMissing = false
			return nil, os.ErrNotExist
		}
		return &maxminddb.Reader{}, nil
	}
	t.Cleanup(func() { maxmindDBOpen = oldMaxMindDBOpen })

	scheduled := make(chan struct{})
	oldScheduleUpdate := scheduleUpdate
	scheduleUpdate = func(*MaxMind, task.Parent) {
		close(scheduled)
	}
	t.Cleanup(func() { scheduleUpdate = oldScheduleUpdate })

	parent := task.RootTask("test", true)
	defer parent.Finish(nil)

	err := cfg.LoadMaxMindDB(parent)
	if err != nil {
		t.Fatalf("loadMaxMindDB() error = %v", err)
	}

	select {
	case <-scheduled:
	case <-time.After(time.Second):
		t.Fatal("expected update scheduler to start")
	}

	if cfg.db.Reader == nil {
		t.Error("expected db instance")
	}
}

func Test_MaxMindConfig_loadMaxMindDBDoesNotScheduleAfterFailedDownload(t *testing.T) {
	cfg := testCfg()
	mockDataDir(t)

	oldIsTest := common.IsTest
	common.IsTest = false
	t.Cleanup(func() { common.IsTest = oldIsTest })

	oldDoReq := doReq
	doReq = func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("download failed")
	}
	t.Cleanup(func() { doReq = oldDoReq })

	scheduled := make(chan struct{})
	oldScheduleUpdate := scheduleUpdate
	scheduleUpdate = func(*MaxMind, task.Parent) {
		close(scheduled)
	}
	t.Cleanup(func() { scheduleUpdate = oldScheduleUpdate })

	parent := task.RootTask("test", true)
	defer parent.Finish(nil)

	if err := cfg.LoadMaxMindDB(parent); err == nil {
		t.Fatal("expected load error")
	}

	select {
	case <-scheduled:
		t.Fatal("scheduler should not start after failed download")
	default:
	}
}

func Test_MaxMindConfig_loadMaxMindDB(t *testing.T) {
	cfg := testCfg()
	mockDataDir(t)
	mockMaxMindDBOpen(t)

	task := task.RootTask("test", true)
	defer task.Finish(nil)
	err := cfg.LoadMaxMindDB(task)
	if err != nil {
		t.Errorf("loadMaxMindDB() error = %v", err)
	}
	if cfg.db.Reader == nil {
		t.Error("expected db instance")
	}
}

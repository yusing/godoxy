package maxmind

import (
	"archive/tar"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-golang"
	"github.com/rs/zerolog"
	maxmind "github.com/yusing/go-proxy/internal/maxmind/types"
	"github.com/yusing/go-proxy/internal/task"
)

// --- Helper for MaxMindConfig ---
type testLogger struct{ zerolog.Logger }

func (testLogger) Info() *zerolog.Event       { return &zerolog.Event{} }
func (testLogger) Warn() *zerolog.Event       { return &zerolog.Event{} }
func (testLogger) Err(_ error) *zerolog.Event { return &zerolog.Event{} }

func testCfg() *MaxMind {
	return &MaxMind{
		Config: &Config{
			AccountID:  "testid",
			LicenseKey: "testkey",
			Database:   maxmind.MaxMindGeoLite,
		},
	}
}

var testLastMod = time.Now().UTC()

func testDoReq(cfg *MaxMind, w http.ResponseWriter, r *http.Request) {
	if u, p, ok := r.BasicAuth(); !ok || u != "testid" || p != "testkey" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Last-Modified", testLastMod.Format(http.TimeFormat))
	gz := gzip.NewWriter(w)
	t := tar.NewWriter(gz)
	t.WriteHeader(&tar.Header{
		Name: cfg.dbFilename(),
	})
	t.Write([]byte("1234"))
	t.Close()
	gz.Close()
	w.WriteHeader(http.StatusOK)
}

func mockDoReq(cfg *MaxMind, t *testing.T) {
	rw := httptest.NewRecorder()
	oldDoReq := doReq
	doReq = func(req *http.Request) (*http.Response, error) {
		testDoReq(cfg, rw, req)
		return rw.Result(), nil
	}
	t.Cleanup(func() { doReq = oldDoReq })
}

func mockDataDir(t *testing.T) {
	oldDataDir := dataDir
	dataDir = t.TempDir()
	t.Cleanup(func() { dataDir = oldDataDir })
}

func mockMaxMindDBOpen(t *testing.T) {
	oldMaxMindDBOpen := maxmindDBOpen
	maxmindDBOpen = func(path string) (*maxminddb.Reader, error) {
		return &maxminddb.Reader{}, nil
	}
	t.Cleanup(func() { maxmindDBOpen = oldMaxMindDBOpen })
}

func Test_MaxMindConfig_doReq(t *testing.T) {
	cfg := testCfg()
	mockDoReq(cfg, t)
	resp, err := cfg.doReq(http.MethodGet)
	if err != nil {
		t.Fatalf("newReq() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected status: %v", resp.StatusCode)
	}
}

func Test_MaxMindConfig_checkLatest(t *testing.T) {
	cfg := testCfg()
	mockDoReq(cfg, t)

	latest, err := cfg.checkLastest()
	if err != nil {
		t.Fatalf("checkLatest() error = %v", err)
	}
	if latest.Equal(testLastMod) {
		t.Errorf("expected latest equal to testLastMod")
	}
}

func Test_MaxMindConfig_download(t *testing.T) {
	cfg := testCfg()
	mockDataDir(t)
	mockMaxMindDBOpen(t)
	mockDoReq(cfg, t)

	err := cfg.download()
	if err != nil {
		t.Fatalf("download() error = %v", err)
	}
	if cfg.db.Reader == nil {
		t.Error("expected db instance")
	}
}

func Test_MaxMindConfig_loadMaxMindDB(t *testing.T) {
	cfg := testCfg()
	mockDataDir(t)
	mockMaxMindDBOpen(t)

	task := task.RootTask("test")
	defer task.Finish(nil)
	err := cfg.LoadMaxMindDB(task)
	if err != nil {
		t.Errorf("loadMaxMindDB() error = %v", err)
	}
	if cfg.db.Reader == nil {
		t.Error("expected db instance")
	}
}

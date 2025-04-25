package acl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-golang"
	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/task"
)

func Test_dbPath(t *testing.T) {
	tmpDataDir := "/tmp/testdata"
	oldDataDir := dataDir
	dataDir = tmpDataDir
	defer func() { dataDir = oldDataDir }()

	tests := []struct {
		name   string
		dbType MaxMindDatabaseType
		want   string
	}{
		{"GeoLite", MaxMindGeoLite, filepath.Join(tmpDataDir, "GeoLite2-City.mmdb")},
		{"GeoIP2", MaxMindGeoIP2, filepath.Join(tmpDataDir, "GeoIP2-City.mmdb")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dbPath(tt.dbType); got != tt.want {
				t.Errorf("dbPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_dbURL(t *testing.T) {
	tests := []struct {
		name   string
		dbType MaxMindDatabaseType
		want   string
	}{
		{"GeoLite", MaxMindGeoLite, "https://download.maxmind.com/geoip/databases/GeoLite2-City/download?suffix=tar.gz"},
		{"GeoIP2", MaxMindGeoIP2, "https://download.maxmind.com/geoip/databases/GeoIP2-City/download?suffix=tar.gz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dbURL(tt.dbType); got != tt.want {
				t.Errorf("dbURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Helper for MaxMindConfig ---
type testLogger struct{ zerolog.Logger }

func (testLogger) Info() *zerolog.Event       { return &zerolog.Event{} }
func (testLogger) Warn() *zerolog.Event       { return &zerolog.Event{} }
func (testLogger) Err(_ error) *zerolog.Event { return &zerolog.Event{} }

func Test_MaxMindConfig_newReq(t *testing.T) {
	cfg := &MaxMindConfig{
		AccountID:  "testid",
		LicenseKey: "testkey",
		Database:   MaxMindGeoLite,
		logger:     zerolog.Nop(),
	}

	// Patch httpClient to use httptest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "testid" || p != "testkey" {
			t.Errorf("basic auth not set correctly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	oldURL := dbURL
	dbURL = func(MaxMindDatabaseType) string { return server.URL }
	defer func() { dbURL = oldURL }()

	resp, err := cfg.newReq(http.MethodGet)
	if err != nil {
		t.Fatalf("newReq() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("unexpected status: %v", resp.StatusCode)
	}
}

func Test_MaxMindConfig_checkUpdate(t *testing.T) {
	cfg := &MaxMindConfig{
		AccountID:  "id",
		LicenseKey: "key",
		Database:   MaxMindGeoLite,
		logger:     zerolog.Nop(),
	}
	lastMod := time.Now().UTC().Format(http.TimeFormat)
	buildTime := time.Now().Add(-time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Last-Modified", lastMod)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	oldURL := dbURL
	dbURL = func(MaxMindDatabaseType) string { return server.URL }
	defer func() { dbURL = oldURL }()

	latest, err := cfg.checkLastest()
	if err != nil {
		t.Fatalf("checkUpdate() error = %v", err)
	}
	if latest.Equal(buildTime) {
		t.Errorf("expected update needed")
	}
}

type fakeReadCloser struct {
	firstRead bool
	closed    bool
}

func (c *fakeReadCloser) Read(p []byte) (int, error) {
	if !c.firstRead {
		c.firstRead = true
		return strings.NewReader("FAKEMMDB").Read(p)
	}
	return 0, io.EOF
}

func (c *fakeReadCloser) Close() error {
	c.closed = true
	return nil
}

func Test_MaxMindConfig_download(t *testing.T) {
	cfg := &MaxMindConfig{
		AccountID:  "id",
		LicenseKey: "key",
		Database:   MaxMindGeoLite,
		logger:     zerolog.Nop(),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, strings.NewReader("FAKEMMDB"))
	}))
	defer server.Close()
	oldURL := dbURL
	dbURL = func(MaxMindDatabaseType) string { return server.URL }
	defer func() { dbURL = oldURL }()

	tmpDir := t.TempDir()
	oldDataDir := dataDir
	dataDir = tmpDir
	defer func() { dataDir = oldDataDir }()

	// Patch maxminddb.Open to always succeed
	origOpen := maxmindDBOpen
	maxmindDBOpen = func(path string) (*maxminddb.Reader, error) {
		return &maxminddb.Reader{}, nil
	}
	defer func() { maxmindDBOpen = origOpen }()

	rw := &fakeReadCloser{}
	oldNewReq := newReq
	newReq = func(cfg *MaxMindConfig, method string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       rw,
		}, nil
	}
	defer func() { newReq = oldNewReq }()

	db, err := cfg.download()
	if err != nil {
		t.Fatalf("download() error = %v", err)
	}
	if db == nil {
		t.Error("expected db instance")
	}
	if !rw.closed {
		t.Error("expected rw to be closed")
	}
}

func Test_MaxMindConfig_loadMaxMindDB(t *testing.T) {
	// This test should cover both the path where DB exists and where it does not
	// For brevity, only the non-existing path is tested here
	cfg := &MaxMindConfig{
		AccountID:  "id",
		LicenseKey: "key",
		Database:   MaxMindGeoLite,
		logger:     zerolog.Nop(),
	}
	oldOpen := maxmindDBOpen
	maxmindDBOpen = func(path string) (*maxminddb.Reader, error) {
		return &maxminddb.Reader{}, nil
	}
	defer func() { maxmindDBOpen = oldOpen }()

	oldDBPath := dbPath
	dbPath = func(MaxMindDatabaseType) string { return filepath.Join(t.TempDir(), "maxmind.mmdb") }
	defer func() { dbPath = oldDBPath }()

	task := task.RootTask("test")
	defer task.Finish(nil)
	err := cfg.LoadMaxMindDB(task)
	if err != nil {
		t.Errorf("loadMaxMindDB() error = %v", err)
	}
}

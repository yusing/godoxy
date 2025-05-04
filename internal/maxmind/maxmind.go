package maxmind

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	maxmind "github.com/yusing/go-proxy/internal/maxmind/types"
	"github.com/yusing/go-proxy/internal/task"
)

type MaxMind struct {
	*Config
	lastUpdate time.Time
	db         struct {
		*maxminddb.Reader
		sync.RWMutex
	}
}

type (
	Config = maxmind.Config
	IPInfo = maxmind.IPInfo
	City   = maxmind.City
)

var (
	updateInterval = 24 * time.Hour
	httpClient     = &http.Client{
		Timeout: 10 * time.Second,
	}
	ErrResponseNotOK   = gperr.New("response not OK")
	ErrDownloadFailure = gperr.New("download failure")
)

func (cfg *MaxMind) dbPath() string {
	if cfg.Database == maxmind.MaxMindGeoLite {
		return filepath.Join(dataDir, "GeoLite2-City.mmdb")
	}
	return filepath.Join(dataDir, "GeoIP2-City.mmdb")
}

func (cfg *MaxMind) dbURL() string {
	if cfg.Database == maxmind.MaxMindGeoLite {
		return "https://download.maxmind.com/geoip/databases/GeoLite2-City/download?suffix=tar.gz"
	}
	return "https://download.maxmind.com/geoip/databases/GeoIP2-City/download?suffix=tar.gz"
}

func (cfg *MaxMind) dbFilename() string {
	if cfg.Database == maxmind.MaxMindGeoLite {
		return "GeoLite2-City.mmdb"
	}
	return "GeoIP2-City.mmdb"
}

func (cfg *MaxMind) LoadMaxMindDB(parent task.Parent) gperr.Error {
	if cfg.Database == "" {
		return nil
	}

	init := parent.Subtask("maxmind_db", true)
	defer init.Finish(nil)

	path := dbPath(cfg)
	reader, err := maxmindDBOpen(path)
	valid := true
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
		default:
			// ignore invalid error, just download it again
			var invalidErr maxminddb.InvalidDatabaseError
			if !errors.As(err, &invalidErr) {
				return gperr.Wrap(err)
			}
		}
		valid = false
	}

	if !valid {
		cfg.Logger().Info().Msg("MaxMind DB not found/invalid, downloading...")
		if err = cfg.download(); err != nil {
			return ErrDownloadFailure.With(err)
		}
	} else {
		cfg.Logger().Info().Msg("MaxMind DB loaded")
		cfg.db.Reader = reader
		go cfg.scheduleUpdate(parent)
	}
	return nil
}

func (cfg *MaxMind) loadLastUpdate() {
	f, err := os.Stat(cfg.dbPath())
	if err != nil {
		return
	}
	cfg.lastUpdate = f.ModTime()
}

func (cfg *MaxMind) setLastUpdate(t time.Time) {
	cfg.lastUpdate = t
	_ = os.Chtimes(cfg.dbPath(), t, t)
}

func (cfg *MaxMind) scheduleUpdate(parent task.Parent) {
	task := parent.Subtask("maxmind_schedule_update", true)
	ticker := time.NewTicker(updateInterval)

	cfg.loadLastUpdate()
	cfg.update()

	defer func() {
		ticker.Stop()
		if cfg.db.Reader != nil {
			cfg.db.Reader.Close()
		}
		task.Finish(nil)
	}()

	for {
		select {
		case <-task.Context().Done():
			return
		case <-ticker.C:
			cfg.update()
		}
	}
}

func (cfg *MaxMind) update() {
	// check for update
	cfg.Logger().Info().Msg("checking for MaxMind DB update...")
	remoteLastModified, err := cfg.checkLastest()
	if err != nil {
		cfg.Logger().Err(err).Msg("failed to check MaxMind DB update")
		return
	}
	if remoteLastModified.Equal(cfg.lastUpdate) {
		cfg.Logger().Info().Msg("MaxMind DB is up to date")
		return
	}

	cfg.Logger().Info().
		Time("latest", remoteLastModified.Local()).
		Time("current", cfg.lastUpdate).
		Msg("MaxMind DB update available")
	if err = cfg.download(); err != nil {
		cfg.Logger().Err(err).Msg("failed to update MaxMind DB")
		return
	}
	cfg.Logger().Info().Msg("MaxMind DB updated")
}

func (cfg *MaxMind) doReq(method string) (*http.Response, error) {
	req, err := http.NewRequest(method, cfg.dbURL(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.AccountID, cfg.LicenseKey)
	resp, err := doReq(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (cfg *MaxMind) checkLastest() (lastModifiedT *time.Time, err error) {
	resp, err := cfg.doReq(http.MethodHead)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrResponseNotOK, resp.StatusCode)
	}

	lastModified := resp.Header.Get("Last-Modified")
	if lastModified == "" {
		cfg.Logger().Warn().Msg("MaxMind responded no last modified time, update skipped")
		return nil, nil
	}

	lastModifiedTime, err := time.Parse(http.TimeFormat, lastModified)
	if err != nil {
		cfg.Logger().Warn().Err(err).Msg("MaxMind responded invalid last modified time, update skipped")
		return nil, err
	}

	return &lastModifiedTime, nil
}

func (cfg *MaxMind) download() error {
	resp, err := cfg.doReq(http.MethodGet)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %d", ErrResponseNotOK, resp.StatusCode)
	}

	dbFile := dbPath(cfg)
	tmpGZPath := dbFile + "-tmp.tar.gz"
	tmpDBPath := dbFile + "-tmp"

	tmpGZFile, err := os.OpenFile(tmpGZPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}

	// cleanup the tar.gz file
	defer func() {
		_ = tmpGZFile.Close()
		_ = os.Remove(tmpGZPath)
	}()

	cfg.Logger().Info().Msg("MaxMind DB downloading...")

	_, err = io.Copy(tmpGZFile, resp.Body)
	if err != nil {
		return err
	}

	if _, err := tmpGZFile.Seek(0, io.SeekStart); err != nil {
		return err
	}

	// extract .tar.gz and to database
	err = extractFileFromTarGz(tmpGZFile, cfg.dbFilename(), tmpDBPath)

	if err != nil {
		return gperr.New("failed to extract database from archive").With(err)
	}

	// test if the downloaded database is valid
	db, err := maxmindDBOpen(tmpDBPath)
	if err != nil {
		_ = os.Remove(tmpDBPath)
		return err
	}

	db.Close()
	err = os.Rename(tmpDBPath, dbFile)
	if err != nil {
		return err
	}

	cfg.db.Lock()
	defer cfg.db.Unlock()
	if cfg.db.Reader != nil {
		cfg.db.Reader.Close()
	}
	cfg.db.Reader, err = maxmindDBOpen(dbFile)
	if err != nil {
		return err
	}

	lastModifiedStr := resp.Header.Get("Last-Modified")
	lastModifiedTime, err := time.Parse(http.TimeFormat, lastModifiedStr)
	if err == nil {
		cfg.setLastUpdate(lastModifiedTime)
	}

	cfg.Logger().Info().Msg("MaxMind DB downloaded")
	return nil
}

func extractFileFromTarGz(tarGzFile *os.File, targetFilename, destPath string) error {
	defer tarGzFile.Close()

	gzr, err := gzip.NewReader(tarGzFile)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return err
		}
		// Only extract the file that matches targetFilename (basename match)
		if filepath.Base(hdr.Name) == targetFilename {
			outFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
			if err != nil {
				return err
			}
			defer outFile.Close()
			_, err = io.Copy(outFile, tr)
			if err != nil {
				return err
			}
			return nil // Done
		}
	}
	return fmt.Errorf("file %s not found in archive", targetFilename)
}

var (
	dataDir       = common.DataDir
	dbPath        = (*MaxMind).dbPath
	doReq         = httpClient.Do
	maxmindDBOpen = maxminddb.Open
)

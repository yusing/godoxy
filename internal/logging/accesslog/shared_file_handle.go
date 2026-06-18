package accesslog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/yusing/goutils/synk"
)

const archiveTimestampLayout = "20060102T150405.000000000Z"

type sharedFileHandle struct {
	*os.File

	// os.File.Name() may not equal to key of `openedFiles`.
	// Store it for later delete from `openedFiles`.
	path string

	refCount *synk.RefCount
}

var (
	openedFiles   = make(map[string]*sharedFileHandle)
	openedFilesMu sync.Mutex
)

// OpenFile creates a new file writer with cleaned path.
//
// If the file is already opened, it will be returned.
func OpenFile(path string) (File, error) {
	openedFilesMu.Lock()
	defer openedFilesMu.Unlock()

	var file *sharedFileHandle
	var err error

	// make it absolute path, so that we can use it as key of `openedFiles` and shared lock
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("access log path error: %w", err)
	}

	if opened, ok := openedFiles[path]; ok {
		opened.refCount.Add()
		return opened, nil
	}

	// cannot open as O_APPEND as we need Seek and WriteAt
	f, err := openAccessLogFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	file = &sharedFileHandle{File: f, path: path, refCount: synk.NewRefCounter()}
	openedFiles[path] = file

	log.Debug().Str("path", path).Msg("file opened")

	go file.closeOnZero()
	return file, nil
}

func (f *sharedFileHandle) Name() string {
	return f.path
}

func (f *sharedFileHandle) RotateActiveLog(now time.Time) (string, error) {
	archivePath, err := f.nextArchivePath(now)
	if err != nil {
		return "", err
	}

	info, err := f.File.Stat()
	if err != nil {
		return "", err
	}
	metadata, err := captureAccessLogMetadata(f.File, info)
	if err != nil {
		return "", err
	}

	err = f.File.Close()
	if err != nil {
		return "", err
	}

	if err = os.Rename(f.path, archivePath); err != nil {
		reopened, openErr := openAccessLogFile(f.path, os.O_CREATE|os.O_RDWR, metadata.mode)
		if openErr == nil {
			f.File = reopened
		}
		return "", errors.Join(err, openErr)
	}

	reopened, err := openAccessLogFile(f.path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, metadata.mode)
	if err != nil {
		rollbackErr := os.Rename(archivePath, f.path)
		restored, restoreErr := openAccessLogFile(f.path, os.O_CREATE|os.O_RDWR, metadata.mode)
		if restoreErr == nil {
			f.File = restored
		}
		return "", errors.Join(err, rollbackErr, restoreErr)
	}
	f.File = reopened
	if err := metadata.apply(f.path); err != nil {
		return "", err
	}
	return archivePath, nil
}

func (f *sharedFileHandle) CleanupRotatedLogs(cutoff time.Time) (int, error) {
	entries, err := os.ReadDir(filepath.Dir(f.path))
	if err != nil {
		return 0, err
	}

	prefix := filepath.Base(f.path) + "."
	var deleted int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) {
			continue
		}
		rotatedAt, ok := parseArchiveTime(strings.TrimPrefix(name, prefix))
		if !ok || !rotatedAt.Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(filepath.Dir(f.path), name)); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func (f *sharedFileHandle) nextArchivePath(now time.Time) (string, error) {
	base := f.path + "." + now.UTC().Format(archiveTimestampLayout)
	path := base
	for i := 1; ; i++ {
		_, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return path, nil
		}
		if err != nil {
			return "", err
		}
		path = fmt.Sprintf("%s.%d", base, i)
	}
}

func parseArchiveTime(suffix string) (time.Time, bool) {
	if len(suffix) < len(archiveTimestampLayout) {
		return time.Time{}, false
	}
	t, err := time.Parse(archiveTimestampLayout, suffix[:len(archiveTimestampLayout)])
	return t, err == nil
}

func openAccessLogFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("access log open error: %w", err)
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return nil, fmt.Errorf("access log seek error: %w", err)
	}
	return f, nil
}

func (f *sharedFileHandle) Close() error {
	f.refCount.Sub()
	return nil
}

func (f *sharedFileHandle) closeOnZero() {
	defer log.Debug().Str("path", f.path).Msg("file closed")

	<-f.refCount.Zero()

	openedFilesMu.Lock()
	delete(openedFiles, f.path)
	openedFilesMu.Unlock()
	err := f.File.Close()
	if err != nil {
		log.Error().Str("path", f.path).Err(err).Msg("failed to close file")
	}
}

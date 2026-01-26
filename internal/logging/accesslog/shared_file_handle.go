package accesslog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/yusing/goutils/synk"
)

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
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("access log open error: %w", err)
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return nil, fmt.Errorf("access log seek error: %w", err)
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

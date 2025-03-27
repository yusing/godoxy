package accesslog

import (
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
)

type File struct {
	*os.File
	sync.Mutex

	// os.File.Name() may not equal to key of `openedFiles`.
	// Store it for later delete from `openedFiles`.
	path string

	refCount *utils.RefCount
}

var (
	openedFiles   = make(map[string]*File)
	openedFilesMu sync.Mutex
)

func NewFileAccessLogger(parent task.Parent, cfg *Config) (*AccessLogger, error) {
	openedFilesMu.Lock()

	var file *File
	path := path.Clean(cfg.Path)
	if opened, ok := openedFiles[path]; ok {
		opened.refCount.Add()
		file = opened
	} else {
		f, err := os.OpenFile(cfg.Path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			openedFilesMu.Unlock()
			return nil, fmt.Errorf("access log open error: %w", err)
		}
		file = &File{File: f, path: path, refCount: utils.NewRefCounter()}
		openedFiles[path] = file
		go file.closeOnZero()
	}

	openedFilesMu.Unlock()
	return NewAccessLogger(parent, file, cfg), nil
}

func (f *File) Close() error {
	f.refCount.Sub()
	return nil
}

func (f *File) closeOnZero() {
	defer logging.Debug().
		Str("path", f.path).
		Msg("access log closed")

	<-f.refCount.Zero()

	openedFilesMu.Lock()
	delete(openedFiles, f.path)
	openedFilesMu.Unlock()
	f.File.Close()
}

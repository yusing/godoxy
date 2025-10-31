package accesslog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/utils"
)

type File struct {
	f *os.File

	// os.File.Name() may not equal to key of `openedFiles`.
	// Store it for later delete from `openedFiles`.
	path string

	refCount *utils.RefCount
}

var (
	openedFiles   = make(map[string]*File)
	openedFilesMu sync.Mutex
)

// NewFileIO creates a new file writer with cleaned path.
//
// If the file is already opened, it will be returned.
func NewFileIO(path string) (Writer, error) {
	openedFilesMu.Lock()
	defer openedFilesMu.Unlock()

	var file *File
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
		return nil, fmt.Errorf("access log seek error: %w", err)
	}
	file = &File{f: f, path: path, refCount: utils.NewRefCounter()}
	openedFiles[path] = file
	go file.closeOnZero()
	return file, nil
}

// Name returns the absolute path of the file.
func (f *File) Name() string {
	return f.path
}

func (f *File) ShouldBeBuffered() bool {
	return true
}

func (f *File) Write(p []byte) (n int, err error) {
	return f.f.Write(p)
}

func (f *File) ReadAt(p []byte, off int64) (n int, err error) {
	return f.f.ReadAt(p, off)
}

func (f *File) WriteAt(p []byte, off int64) (n int, err error) {
	return f.f.WriteAt(p, off)
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	return f.f.Seek(offset, whence)
}

func (f *File) Size() (int64, error) {
	stat, err := f.f.Stat()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (f *File) Truncate(size int64) error {
	return f.f.Truncate(size)
}

func (f *File) Close() error {
	f.refCount.Sub()
	return nil
}

func (f *File) closeOnZero() {
	defer log.Debug().
		Str("path", f.path).
		Msg("access log closed")

	<-f.refCount.Zero()

	openedFilesMu.Lock()
	delete(openedFiles, f.path)
	openedFilesMu.Unlock()
	f.f.Close()
}

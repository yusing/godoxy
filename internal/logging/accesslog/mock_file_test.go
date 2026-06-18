package accesslog_test

import (
	"bytes"
	"io"
	"io/fs"
	"time"

	. "github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/goutils/task"
)

type mockFile struct {
	name string
	data []byte
	pos  int64
}

var _ File = (*mockFile)(nil)

func newMockFile(_ bool) *mockFile {
	return &mockFile{name: "accesslog-test"}
}

func (m *mockFile) Len() int64 {
	return int64(len(m.data))
}

func (m *mockFile) Content() []byte {
	return bytes.Clone(m.data)
}

func (m *mockFile) NumLines() int {
	content := m.Content()
	count := bytes.Count(content, []byte("\n"))
	if len(content) > 0 && content[len(content)-1] != '\n' {
		count++
	}
	return count
}

func (m *mockFile) MustSize() int64 {
	return m.Len()
}

func (m *mockFile) Name() string {
	return m.name
}

func (m *mockFile) Write(p []byte) (int, error) {
	if m.pos < 0 {
		return 0, fs.ErrInvalid
	}
	end := m.pos + int64(len(p))
	if end > int64(len(m.data)) {
		m.data = append(m.data, make([]byte, end-int64(len(m.data)))...)
	}
	copy(m.data[m.pos:end], p)
	m.pos = end
	return len(p), nil
}

func (m *mockFile) WriteAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fs.ErrInvalid
	}
	end := off + int64(len(p))
	if end > int64(len(m.data)) {
		m.data = append(m.data, make([]byte, end-int64(len(m.data)))...)
	}
	copy(m.data[off:end], p)
	return len(p), nil
}

func (m *mockFile) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fs.ErrInvalid
	}
	if off >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n := copy(p, m.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (m *mockFile) Seek(offset int64, whence int) (int64, error) {
	var pos int64
	switch whence {
	case io.SeekStart:
		pos = offset
	case io.SeekCurrent:
		pos = m.pos + offset
	case io.SeekEnd:
		pos = int64(len(m.data)) + offset
	default:
		return 0, fs.ErrInvalid
	}
	if pos < 0 {
		return 0, fs.ErrInvalid
	}
	m.pos = pos
	return pos, nil
}

func (m *mockFile) Truncate(size int64) error {
	if size < 0 {
		return fs.ErrInvalid
	}
	if size <= int64(len(m.data)) {
		m.data = m.data[:size]
	} else {
		m.data = append(m.data, make([]byte, size-int64(len(m.data)))...)
	}
	if m.pos > size {
		m.pos = size
	}
	return nil
}

func (m *mockFile) Stat() (fs.FileInfo, error) {
	return mockFileInfo{name: m.name, size: int64(len(m.data))}, nil
}

func (m *mockFile) Close() error {
	return nil
}

func newMockAccessLogger(parent task.Parent, cfg *RequestLoggerConfig) AccessLogger {
	return NewFileAccessLogger(parent, newMockFile(true), cfg)
}

type mockFileInfo struct {
	name string
	size int64
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return m.size }
func (m mockFileInfo) Mode() fs.FileMode  { return 0o600 }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return false }
func (m mockFileInfo) Sys() any           { return nil }

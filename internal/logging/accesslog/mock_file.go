package accesslog

import (
	"bytes"
	"io"

	"github.com/spf13/afero"
)

type noLock struct{}

func (noLock) Lock()   {}
func (noLock) Unlock() {}

type MockFile struct {
	afero.File
	noLock
}

func NewMockFile() *MockFile {
	f, _ := afero.TempFile(afero.NewMemMapFs(), "", "")
	return &MockFile{
		File: f,
	}
}

func (m *MockFile) Len() int64 {
	filesize, _ := m.Seek(0, io.SeekEnd)
	_, _ = m.Seek(0, io.SeekStart)
	return filesize
}

func (m *MockFile) Content() []byte {
	buf := bytes.NewBuffer(nil)
	m.Seek(0, io.SeekStart)
	_, _ = buf.ReadFrom(m.File)
	m.Seek(0, io.SeekStart)
	return buf.Bytes()
}

func (m *MockFile) NumLines() int {
	content := m.Content()
	count := bytes.Count(content, []byte("\n"))
	// account for last line if it does not end with a newline
	if len(content) > 0 && content[len(content)-1] != '\n' {
		count++
	}
	return count
}

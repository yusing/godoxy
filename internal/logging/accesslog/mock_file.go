package accesslog

import (
	"bytes"
	"io"

	"github.com/spf13/afero"
)

type MockFile struct {
	afero.File

	buffered bool
}

var _ SupportRotate = (*MockFile)(nil)

func NewMockFile(buffered bool) *MockFile {
	f, _ := afero.TempFile(afero.NewMemMapFs(), "", "")
	f.Seek(0, io.SeekEnd)
	return &MockFile{
		File:     f,
		buffered: buffered,
	}
}

func (m *MockFile) ShouldBeBuffered() bool {
	return m.buffered
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

func (m *MockFile) Size() (int64, error) {
	stat, _ := m.Stat()
	return stat.Size(), nil
}

func (m *MockFile) MustSize() int64 {
	size, _ := m.Size()
	return size
}

func (m *MockFile) Close() error {
	return nil
}

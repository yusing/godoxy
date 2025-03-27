package accesslog

import (
	"bytes"
	"io"
	"sync"
)

type MockFile struct {
	data     []byte
	position int64
	sync.Mutex
}

func (m *MockFile) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		m.position = offset
	case io.SeekCurrent:
		m.position += offset
	case io.SeekEnd:
		m.position = int64(len(m.data)) + offset
	}
	return m.position, nil
}

func (m *MockFile) Write(p []byte) (n int, err error) {
	m.data = append(m.data, p...)
	n = len(p)
	m.position += int64(n)
	return
}

func (m *MockFile) Name() string {
	return "mock"
}

func (m *MockFile) Read(p []byte) (n int, err error) {
	if m.position >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.position:])
	m.position += int64(n)
	return n, nil
}

func (m *MockFile) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n = copy(p, m.data[off:])
	return n, nil
}

func (m *MockFile) Close() error {
	return nil
}

func (m *MockFile) Truncate(size int64) error {
	m.data = m.data[:size]
	m.position = size
	return nil
}

func (m *MockFile) LineCount() int {
	m.Lock()
	defer m.Unlock()
	return bytes.Count(m.data[:m.position], []byte("\n"))
}

func (m *MockFile) Len() int64 {
	return m.position
}

func (m *MockFile) Content() []byte {
	return m.data[:m.position]
}

package websocket

import (
	"io"
	"time"
)

type Reader struct {
	manager *Manager
}

func (m *Manager) NewReader() io.Reader {
	return &Reader{
		manager: m,
	}
}

func (r *Reader) Read(p []byte) (int, error) {
	data, err := r.manager.ReadBinary(10 * time.Second)
	if err != nil {
		return 0, err
	}
	copy(p, data)
	return len(data), nil
}

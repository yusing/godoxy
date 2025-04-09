package homepage

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

type content struct {
	header http.Header
	data   []byte
	status int
}

func newContent() *content {
	return &content{
		header: make(http.Header),
	}
}

func (c *content) Header() http.Header {
	return c.header
}

func (c *content) Write(data []byte) (int, error) {
	c.data = append(c.data, data...)
	return len(data), nil
}

func (c *content) WriteHeader(statusCode int) {
	c.status = statusCode
}

func (c *content) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("not supported")
}

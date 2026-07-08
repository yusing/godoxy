package accesslog

import (
	"io"
)

type BufferedWriter interface {
	io.Writer
	io.Closer
	Flush() error
	Resize(size int) error
}

package rules

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"sync"

	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	gperr "github.com/yusing/goutils/errs"
)

type noopWriteCloser struct {
	io.Writer
}

func (n noopWriteCloser) Close() error {
	return nil
}

var (
	stdout io.WriteCloser = noopWriteCloser{os.Stdout}
	stderr io.WriteCloser = noopWriteCloser{os.Stderr}
)

var (
	testFiles     = make(map[string]*bytes.Buffer)
	testFilesLock sync.Mutex
)

func openFile(path string) (io.WriteCloser, gperr.Error) {
	switch path {
	case "/dev/stdout":
		return stdout, nil
	case "/dev/stderr":
		return stderr, nil
	}

	if common.IsTest {
		testFilesLock.Lock()
		defer testFilesLock.Unlock()
		if buf, ok := testFiles[path]; ok {
			return noopWriteCloser{buf}, nil
		}
		buf := bytes.NewBuffer(nil)
		testFiles[path] = buf
		return noopWriteCloser{buf}, nil
	}

	f, err := accesslog.OpenFile(path)
	if err != nil {
		return nil, ErrInvalidArguments.With(err)
	}
	return f, nil
}

func TestRandomFileName() string {
	return fmt.Sprintf("test-file-%d.txt", rand.Intn(1000000))
}

func TestFileContent(path string) []byte {
	testFilesLock.Lock()
	defer testFilesLock.Unlock()

	buf, ok := testFiles[path]
	if !ok {
		return nil
	}
	return buf.Bytes()
}

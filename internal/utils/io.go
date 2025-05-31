package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"syscall"

	"github.com/yusing/go-proxy/internal/utils/synk"
)

// TODO: move to "utils/io".
type (
	FileReader struct {
		Path string
	}

	ContextReader struct {
		ctx context.Context
		io.Reader
	}

	ContextWriter struct {
		ctx context.Context
		io.Writer
	}

	Pipe struct {
		r ContextReader
		w ContextWriter
	}

	BidirectionalPipe struct {
		pSrcDst *Pipe
		pDstSrc *Pipe
	}
)

func NewContextReader(ctx context.Context, r io.Reader) *ContextReader {
	return &ContextReader{ctx: ctx, Reader: r}
}

func NewContextWriter(ctx context.Context, w io.Writer) *ContextWriter {
	return &ContextWriter{ctx: ctx, Writer: w}
}

func (r *ContextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.Reader.Read(p)
	}
}

func (w *ContextWriter) Write(p []byte) (int, error) {
	select {
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	default:
		return w.Writer.Write(p)
	}
}

func NewPipe(ctx context.Context, r io.ReadCloser, w io.WriteCloser) *Pipe {
	return &Pipe{
		r: ContextReader{ctx: ctx, Reader: r},
		w: ContextWriter{ctx: ctx, Writer: w},
	}
}

func (p *Pipe) Start() (err error) {
	err = CopyClose(&p.w, &p.r, 0)
	switch {
	case
		// NOTE: ignoring broken pipe and connection reset by peer
		errors.Is(err, syscall.EPIPE),
		errors.Is(err, syscall.ECONNRESET):
		return nil
	}
	return err
}

func NewBidirectionalPipe(ctx context.Context, rw1 io.ReadWriteCloser, rw2 io.ReadWriteCloser) BidirectionalPipe {
	return BidirectionalPipe{
		pSrcDst: NewPipe(ctx, rw1, rw2),
		pDstSrc: NewPipe(ctx, rw2, rw1),
	}
}

func (p BidirectionalPipe) Start() error {
	var wg sync.WaitGroup
	wg.Add(2)
	var srcErr, dstErr error
	go func() {
		srcErr = p.pSrcDst.Start()
		wg.Done()
	}()
	go func() {
		dstErr = p.pDstSrc.Start()
		wg.Done()
	}()
	wg.Wait()
	return errors.Join(srcErr, dstErr)
}

type flushErrorInterface interface {
	FlushError() error
}

type flusherWrapper struct {
	rw http.Flusher
}

type rwUnwrapper interface {
	Unwrap() http.ResponseWriter
}

func (f *flusherWrapper) FlushError() error {
	f.rw.Flush()
	return nil
}

func getHTTPFlusher(dst io.Writer) flushErrorInterface {
	// pre-unwrap the flusher to prevent unwrap and check in every loop
	if rw, ok := dst.(http.ResponseWriter); ok {
		for {
			switch t := rw.(type) {
			case flushErrorInterface:
				return t
			case http.Flusher:
				return &flusherWrapper{rw: t}
			case rwUnwrapper:
				rw = t.Unwrap()
			default:
				return nil
			}
		}
	}
	return nil
}

const copyBufSize = synk.SizedPoolThreshold

var bytesPool = synk.NewBytesPool()

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// This is a copy of io.Copy with context and HTTP flusher handling
// Author: yusing <yusing@6uo.me>.
func CopyClose(dst *ContextWriter, src *ContextReader, sizeHint int) (err error) {
	size := copyBufSize
	if l, ok := src.Reader.(*io.LimitedReader); ok {
		if int64(size) > l.N {
			if l.N < 1 {
				size = 1
			} else {
				size = int(l.N)
			}
		}
	} else if sizeHint > 0 {
		size = min(size, sizeHint)
	}
	buf := bytesPool.GetSized(size)
	defer bytesPool.Put(buf)
	// close both as soon as one of them is done
	wCloser, wCanClose := dst.Writer.(io.Closer)
	rCloser, rCanClose := src.Reader.(io.Closer)
	if wCanClose || rCanClose {
		go func() {
			select {
			case <-src.ctx.Done():
			case <-dst.ctx.Done():
			}
			if rCanClose {
				defer rCloser.Close()
			}
			if wCanClose {
				defer wCloser.Close()
			}
		}()
	}
	flusher := getHTTPFlusher(dst.Writer)
	for {
		nr, er := src.Reader.Read(buf)
		if nr > 0 {
			nw, ew := dst.Writer.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			if ew != nil {
				err = ew
				return
			}
			if nr != nw {
				err = io.ErrShortWrite
				return
			}
			if flusher != nil {
				err = flusher.FlushError()
				if err != nil {
					return err
				}
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			return
		}
	}
}

func CopyCloseWithContext(ctx context.Context, dst io.Writer, src io.Reader, sizeHint int) (err error) {
	return CopyClose(NewContextWriter(ctx, dst), NewContextReader(ctx, src), sizeHint)
}

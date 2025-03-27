package utils

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"syscall"

	"github.com/yusing/go-proxy/internal/gperr"
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
	err = CopyClose(&p.w, &p.r)
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

func (p BidirectionalPipe) Start() gperr.Error {
	var wg sync.WaitGroup
	wg.Add(2)
	b := gperr.NewBuilder("bidirectional pipe error")
	go func() {
		b.Add(p.pSrcDst.Start())
		wg.Done()
	}()
	go func() {
		b.Add(p.pDstSrc.Start())
		wg.Done()
	}()
	wg.Wait()
	return b.Error()
}

var copyBufPool = sync.Pool{
	New: func() any {
		return make([]byte, copyBufSize)
	},
}

type httpFlusher interface {
	Flush() error
}

func getHttpFlusher(dst io.Writer) httpFlusher {
	if rw, ok := dst.(http.ResponseWriter); ok {
		return http.NewResponseController(rw)
	}
	return nil
}

const (
	copyBufSize = 32 * 1024
)

var copyBufPool = sync.Pool{
	New: func() any {
		return make([]byte, copyBufSize)
	},
}

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// This is a copy of io.Copy with context and HTTP flusher handling
// Author: yusing <yusing@6uo.me>.
func CopyClose(dst *ContextWriter, src *ContextReader) (err error) {
	var buf []byte
	if l, ok := src.Reader.(*io.LimitedReader); ok {
		size := copyBufSize
		if int64(size) > l.N {
			if l.N < 1 {
				size = 1
			} else {
				size = int(l.N)
			}
		}
		buf = make([]byte, 0, size)
	} else {
		buf = copyBufPool.Get().([]byte)
		defer copyBufPool.Put(buf)
	}
	// close both as soon as one of them is done
	wCloser, wCanClose := dst.Writer.(io.Closer)
	rCloser, rCanClose := src.Reader.(io.Closer)
	if wCanClose || rCanClose {
		if src.ctx == dst.ctx {
			go func() {
				<-src.ctx.Done()
				if wCanClose {
					wCloser.Close()
				}
				if rCanClose {
					rCloser.Close()
				}
			}()
		} else {
			if wCloser != nil {
				go func() {
					<-src.ctx.Done()
					wCloser.Close()
				}()
			}
			if rCloser != nil {
				go func() {
					<-dst.ctx.Done()
					rCloser.Close()
				}()
			}
		}
	}
	flusher := getHttpFlusher(dst.Writer)
	canFlush := flusher != nil
	for {
		nr, er := src.Reader.Read(buf[:copyBufSize])
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
			if canFlush {
				err = flusher.Flush()
				if err != nil {
					if errors.Is(err, http.ErrNotSupported) {
						canFlush = false
						err = nil
					} else {
						return err
					}
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

func CopyCloseWithContext(ctx context.Context, dst io.Writer, src io.Reader) (err error) {
	return CopyClose(NewContextWriter(ctx, dst), NewContextReader(ctx, src))
}

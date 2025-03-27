package utils

import (
	"context"
	"errors"
	"io"
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
	err = Copy(&p.w, &p.r)
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

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// This is a copy of io.Copy with context handling
// Author: yusing <yusing@6uo.me>.
func Copy(dst *ContextWriter, src *ContextReader) (err error) {
	size := 32 * 1024
	if l, ok := src.Reader.(*io.LimitedReader); ok && int64(size) > l.N {
		if l.N < 1 {
			size = 1
		} else {
			size = int(l.N)
		}
	}
	buf := make([]byte, size)
	for {
		select {
		case <-src.ctx.Done():
			return src.ctx.Err()
		case <-dst.ctx.Done():
			return dst.ctx.Err()
		default:
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
			}
			if er != nil {
				if er != io.EOF {
					err = er
				}
				return
			}
		}
	}
}

func Copy2(ctx context.Context, dst io.Writer, src io.Reader) error {
	return Copy(&ContextWriter{ctx: ctx, Writer: dst}, &ContextReader{ctx: ctx, Reader: src})
}

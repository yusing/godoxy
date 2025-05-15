// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Modified from bufio.Writer by yusing <yusing@6uo.me>.
package utils

import (
	"io"
	"unicode/utf8"
)

// buffered output

// BufferedWriter implements buffering for an [io.BufferedWriter] object.
// If an error occurs writing to a [BufferedWriter], no more data will be
// accepted and all subsequent writes, and [BufferedWriter.Flush], will return the error.
// After all data has been written, the client should call the
// [BufferedWriter.Flush] method to guarantee all data has been forwarded to
// the underlying [io.BufferedWriter].
type BufferedWriter struct {
	err error
	buf []byte
	n   int
	wr  io.Writer
}

// NewBufferedWriter returns a new [BufferedWriter] whose buffer has at least the specified
// size. If the argument io.Writer is already a [BufferedWriter] with large enough
// size, it returns the underlying [BufferedWriter].
func NewBufferedWriter(w io.Writer, size int) *BufferedWriter {
	// Is it already a Writer?
	b, ok := w.(*BufferedWriter)
	if ok && len(b.buf) >= size {
		return b
	}
	return &BufferedWriter{
		buf: bytesPool.GetSized(size),
		wr:  w,
	}
}

// Size returns the size of the underlying buffer in bytes.
func (b *BufferedWriter) Size() int { return len(b.buf) }

func (b *BufferedWriter) Resize(size int) error {
	err := b.Flush()
	if err != nil {
		return err
	}
	if cap(b.buf) >= size {
		b.buf = b.buf[:size]
	} else {
		b.Release()
		b.buf = bytesPool.GetSized(size)
	}
	b.err = nil
	b.n = 0
	return nil
}

func (b *BufferedWriter) Release() {
	bytesPool.Put(b.buf)
}

// Flush writes any buffered data to the underlying [io.Writer].
func (b *BufferedWriter) Flush() error {
	if b.err != nil {
		return b.err
	}
	if b.n == 0 {
		return nil
	}
	n, err := b.wr.Write(b.buf[0:b.n])
	if n < b.n && err == nil {
		err = io.ErrShortWrite
	}
	if err != nil {
		if n > 0 && n < b.n {
			copy(b.buf[0:b.n-n], b.buf[n:b.n])
		}
		b.n -= n
		b.err = err
		return err
	}
	b.n = 0
	return nil
}

// Available returns how many bytes are unused in the buffer.
func (b *BufferedWriter) Available() int { return len(b.buf) - b.n }

// AvailableBuffer returns an empty buffer with b.Available() capacity.
// This buffer is intended to be appended to and
// passed to an immediately succeeding [BufferedWriter.Write] call.
// The buffer is only valid until the next write operation on b.
func (b *BufferedWriter) AvailableBuffer() []byte {
	return b.buf[b.n:][:0]
}

// Buffered returns the number of bytes that have been written into the current buffer.
func (b *BufferedWriter) Buffered() int { return b.n }

// Write writes the contents of p into the buffer.
// It returns the number of bytes written.
// If nn < len(p), it also returns an error explaining
// why the write is short.
func (b *BufferedWriter) Write(p []byte) (nn int, err error) {
	for len(p) > b.Available() && b.err == nil {
		var n int
		if b.Buffered() == 0 {
			// Large write, empty buffer.
			// Write directly from p to avoid copy.
			n, b.err = b.wr.Write(p)
		} else {
			n = copy(b.buf[b.n:], p)
			b.n += n
			b.Flush()
		}
		nn += n
		p = p[n:]
	}
	if b.err != nil {
		return nn, b.err
	}
	n := copy(b.buf[b.n:], p)
	b.n += n
	nn += n
	return nn, nil
}

// WriteByte writes a single byte.
func (b *BufferedWriter) WriteByte(c byte) error {
	if b.err != nil {
		return b.err
	}
	if b.Available() <= 0 && b.Flush() != nil {
		return b.err
	}
	b.buf[b.n] = c
	b.n++
	return nil
}

// WriteRune writes a single Unicode code point, returning
// the number of bytes written and any error.
func (b *BufferedWriter) WriteRune(r rune) (size int, err error) {
	// Compare as uint32 to correctly handle negative runes.
	if uint32(r) < utf8.RuneSelf {
		err = b.WriteByte(byte(r))
		if err != nil {
			return 0, err
		}
		return 1, nil
	}
	if b.err != nil {
		return 0, b.err
	}
	n := b.Available()
	if n < utf8.UTFMax {
		if b.Flush(); b.err != nil {
			return 0, b.err
		}
		n = b.Available()
		if n < utf8.UTFMax {
			// Can only happen if buffer is silly small.
			return b.WriteString(string(r))
		}
	}
	size = utf8.EncodeRune(b.buf[b.n:], r)
	b.n += size
	return size, nil
}

// WriteString writes a string.
// It returns the number of bytes written.
// If the count is less than len(s), it also returns an error explaining
// why the write is short.
func (b *BufferedWriter) WriteString(s string) (int, error) {
	var sw io.StringWriter
	tryStringWriter := true

	nn := 0
	for len(s) > b.Available() && b.err == nil {
		var n int
		if b.Buffered() == 0 && sw == nil && tryStringWriter {
			// Check at most once whether b.wr is a StringWriter.
			sw, tryStringWriter = b.wr.(io.StringWriter)
		}
		if b.Buffered() == 0 && tryStringWriter {
			// Large write, empty buffer, and the underlying writer supports
			// WriteString: forward the write to the underlying StringWriter.
			// This avoids an extra copy.
			n, b.err = sw.WriteString(s)
		} else {
			n = copy(b.buf[b.n:], s)
			b.n += n
			b.Flush()
		}
		nn += n
		s = s[n:]
	}
	if b.err != nil {
		return nn, b.err
	}
	n := copy(b.buf[b.n:], s)
	b.n += n
	nn += n
	return nn, nil
}

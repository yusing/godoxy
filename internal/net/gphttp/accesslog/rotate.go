package accesslog

import (
	"bytes"
	"io"
	"time"
)

func (l *AccessLogger) rotate() (err error) {
	// Get retention configuration
	config := l.Config().Retention
	var shouldKeep func(t time.Time, lineCount int) bool

	if config.Last > 0 {
		shouldKeep = func(_ time.Time, lineCount int) bool {
			return lineCount < int(config.Last)
		}
	} else if config.Days > 0 {
		cutoff := time.Now().AddDate(0, 0, -int(config.Days))
		shouldKeep = func(t time.Time, _ int) bool {
			return !t.IsZero() && !t.Before(cutoff)
		}
	} else {
		return nil // No retention policy set
	}

	s := NewBackScanner(l.io, defaultChunkSize)
	nRead := 0
	nLines := 0
	for s.Scan() {
		nRead += len(s.Bytes()) + 1
		nLines++
		t := ParseLogTime(s.Bytes())
		if !shouldKeep(t, nLines) {
			break
		}
	}
	if s.Err() != nil {
		return s.Err()
	}

	beg := int64(nRead)
	if _, err := l.io.Seek(-beg, io.SeekEnd); err != nil {
		return err
	}
	buf := make([]byte, nRead)
	if _, err := l.io.Read(buf); err != nil {
		return err
	}

	if err := l.writeTruncate(buf); err != nil {
		return err
	}
	return nil
}

func (l *AccessLogger) writeTruncate(buf []byte) (err error) {
	// Seek to beginning and truncate
	if _, err := l.io.Seek(0, 0); err != nil {
		return err
	}

	// Write buffer back to file
	nWritten, err := l.buffered.Write(buf)
	if err != nil {
		return err
	}
	if err = l.buffered.Flush(); err != nil {
		return err
	}

	// Truncate file
	if err = l.io.Truncate(int64(nWritten)); err != nil {
		return err
	}

	// check bytes written == buffer size
	if nWritten != len(buf) {
		return io.ErrShortWrite
	}
	return
}

const timeLen = len(`"time":"`)

var timeJSON = []byte(`"time":"`)

func ParseLogTime(line []byte) (t time.Time) {
	if len(line) == 0 {
		return
	}

	if i := bytes.Index(line, timeJSON); i != -1 { // JSON format
		var jsonStart = i + timeLen
		var jsonEnd = i + timeLen + len(LogTimeFormat)
		if len(line) < jsonEnd {
			return
		}
		timeStr := line[jsonStart:jsonEnd]
		t, _ = time.Parse(LogTimeFormat, string(timeStr))
		return
	}

	// Common/Combined format
	// Format: <virtual host> <host ip> - - [02/Jan/2006:15:04:05 -0700] ...
	start := bytes.IndexByte(line, '[')
	if start == -1 {
		return
	}
	end := bytes.IndexByte(line[start:], ']')
	if end == -1 {
		return
	}
	end += start // adjust end position relative to full line

	timeStr := line[start+1 : end]
	t, _ = time.Parse(LogTimeFormat, string(timeStr)) // ignore error
	return
}

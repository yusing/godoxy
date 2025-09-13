package accesslog

import (
	"bytes"
	"io"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"github.com/yusing/go-proxy/internal/utils/synk"
)

type supportRotate interface {
	io.Seeker
	io.ReaderAt
	io.WriterAt
	Truncate(size int64) error
	Size() (int64, error)
}

type RotateResult struct {
	Filename        string
	OriginalSize    int64 // original size of the file
	NumBytesRead    int64 // number of bytes read from the file
	NumBytesKeep    int64 // number of bytes to keep
	NumLinesRead    int   // number of lines read from the file
	NumLinesKeep    int   // number of lines to keep
	NumLinesInvalid int   // number of invalid lines
}

func (r *RotateResult) Print(logger *zerolog.Logger) {
	event := logger.Info().
		Str("original_size", strutils.FormatByteSize(r.OriginalSize))
	if r.NumBytesRead > 0 {
		event.Str("bytes_read", strutils.FormatByteSize(r.NumBytesRead))
	}
	if r.NumBytesKeep > 0 {
		event.Str("bytes_keep", strutils.FormatByteSize(r.NumBytesKeep))
	}
	if r.NumLinesRead > 0 {
		event.Int("lines_read", r.NumLinesRead)
	}
	if r.NumLinesKeep > 0 {
		event.Int("lines_keep", r.NumLinesKeep)
	}
	if r.NumLinesInvalid > 0 {
		event.Int("lines_invalid", r.NumLinesInvalid)
	}
	event.Str("saved", strutils.FormatByteSize(r.OriginalSize-r.NumBytesKeep)).
		Msg("log rotate result")
}

func (r *RotateResult) Add(other *RotateResult) {
	r.OriginalSize += other.OriginalSize
	r.NumBytesRead += other.NumBytesRead
	r.NumBytesKeep += other.NumBytesKeep
	r.NumLinesRead += other.NumLinesRead
	r.NumLinesKeep += other.NumLinesKeep
	r.NumLinesInvalid += other.NumLinesInvalid
}

type lineInfo struct {
	Pos  int64 // Position from the start of the file
	Size int64 // Size of this line
}

var rotateBytePool = synk.GetBytesPoolWithUniqueMemory()

// rotateLogFile rotates the log file based on the retention policy.
// It returns the result of the rotation and an error if any.
//
// The file is rotated by reading the file backward line-by-line
// and stop once error occurs or found a line that should not be kept.
//
// Any invalid lines will be skipped and not included in the result.
//
// If the file does not need to be rotated, it returns nil, nil.
func rotateLogFile(file supportRotate, config *Retention) (result *RotateResult, err error) {
	if config.KeepSize > 0 {
		result, err = rotateLogFileBySize(file, config)
	} else {
		result, err = rotateLogFileByPolicy(file, config)
	}

	if err != nil {
		return nil, err
	}

	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return nil, err
	}

	return result, nil
}

func rotateLogFileByPolicy(file supportRotate, config *Retention) (result *RotateResult, err error) {
	var shouldStop func() bool
	t := utils.TimeNow()

	if config.Last > 0 {
		shouldStop = func() bool { return result.NumLinesKeep-result.NumLinesInvalid == int(config.Last) }
		// not needed to parse time for last N lines
	} else if config.Days > 0 {
		cutoff := utils.TimeNow().AddDate(0, 0, -int(config.Days)+1)
		shouldStop = func() bool { return t.Before(cutoff) }
	} else {
		return nil, nil // should not happen
	}

	fileSize, err := file.Size()
	if err != nil {
		return nil, err
	}

	// nothing to rotate, return the nothing
	if fileSize == 0 {
		return nil, nil
	}

	s := NewBackScanner(file, fileSize, defaultChunkSize)
	result = &RotateResult{
		OriginalSize: fileSize,
	}

	// Store the line positions and sizes we want to keep
	linesToKeep := make([]lineInfo, 0)
	lastLineValid := false

	for s.Scan() {
		result.NumLinesRead++
		lineSize := int64(len(s.Bytes()) + 1) // +1 for newline
		linePos := result.OriginalSize - result.NumBytesRead - lineSize
		result.NumBytesRead += lineSize

		// Check if line has valid time
		t = ParseLogTime(s.Bytes())
		if t.IsZero() {
			result.NumLinesInvalid++
			lastLineValid = false
			continue
		}

		// Check if we should stop based on retention policy
		if shouldStop() {
			break
		}

		// Add line to those we want to keep
		if lastLineValid {
			last := linesToKeep[len(linesToKeep)-1]
			linesToKeep[len(linesToKeep)-1] = lineInfo{
				Pos:  last.Pos - lineSize,
				Size: last.Size + lineSize,
			}
		} else {
			linesToKeep = append(linesToKeep, lineInfo{
				Pos:  linePos,
				Size: lineSize,
			})
		}
		result.NumBytesKeep += lineSize
		result.NumLinesKeep++
		lastLineValid = true
	}

	if s.Err() != nil {
		return nil, s.Err()
	}

	// nothing to keep, truncate to empty
	if len(linesToKeep) == 0 {
		return nil, file.Truncate(0)
	}

	// nothing to rotate, return nothing
	if result.NumBytesKeep == result.OriginalSize {
		return nil, nil
	}

	// Read each line and write it to the beginning of the file
	writePos := int64(0)
	buf := rotateBytePool.Get()
	defer rotateBytePool.Put(buf)

	// in reverse order to keep the order of the lines (from old to new)
	for i := len(linesToKeep) - 1; i >= 0; i-- {
		line := linesToKeep[i]
		n := line.Size
		if cap(buf) < int(n) {
			buf = make([]byte, n)
		}
		buf = buf[:n]

		// Read the line from its original position
		if _, err := file.ReadAt(buf, line.Pos); err != nil {
			return nil, err
		}

		// Write it to the new position
		if _, err := file.WriteAt(buf, writePos); err != nil {
			return nil, err
		} else if n < line.Size {
			return nil, gperr.Errorf("%w, writing %d bytes, only %d written", io.ErrShortWrite, line.Size, n)
		}
		writePos += n
	}

	if err := file.Truncate(writePos); err != nil {
		return nil, err
	}

	return result, nil
}

// rotateLogFileBySize rotates the log file by size.
// It returns the result of the rotation and an error if any.
//
// The file is not being read, it just truncate the file to the new size.
//
// Invalid lines will not be detected and included in the result.
func rotateLogFileBySize(file supportRotate, config *Retention) (result *RotateResult, err error) {
	filesize, err := file.Size()
	if err != nil {
		return nil, err
	}

	result = &RotateResult{
		OriginalSize: filesize,
	}

	keepSize := int64(config.KeepSize)
	if keepSize >= filesize {
		result.NumBytesKeep = filesize
		return result, nil
	}
	result.NumBytesKeep = keepSize

	err = file.Truncate(keepSize)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ParseLogTime parses the time from the log line.
// It returns the time if the time is found and valid in the log line,
// otherwise it returns zero time.
func ParseLogTime(line []byte) (t time.Time) {
	if len(line) == 0 {
		return
	}

	if timeStr := ExtractTime(line); timeStr != nil {
		t, _ = time.Parse(LogTimeFormat, string(timeStr)) // ignore error
		return
	}
	return
}

var timeJSON = []byte(`"time":"`)

// ExtractTime extracts the time from the log line.
// It returns the time if the time is found,
// otherwise it returns nil.
//
// The returned time is not validated.
func ExtractTime(line []byte) []byte {
	switch line[0] {
	case '{': // JSON format
		if i := bytes.Index(line, timeJSON); i != -1 {
			var jsonStart = i + len(`"time":"`)
			var jsonEnd = i + len(`"time":"`) + len(LogTimeFormat)
			if len(line) < jsonEnd {
				return nil
			}
			return line[jsonStart:jsonEnd]
		}
		return nil // invalid JSON line
	default:
		// Common/Combined format
		// Format: <virtual host> <host ip> - - [02/Jan/2006:15:04:05 -0700] ...
		start := bytes.IndexByte(line, '[')
		if start == -1 {
			return nil
		}
		end := start + 1 + len(LogTimeFormat)
		if len(line) < end {
			return nil
		}
		return line[start+1 : end]
	}
}

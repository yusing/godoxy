package accesslog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/goutils/mockable"
	strutils "github.com/yusing/goutils/strings"
)

type supportRotate interface {
	io.Seeker
	io.ReaderAt
	io.WriterAt
	Truncate(size int64) error
	Stat() (fs.FileInfo, error)
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

type lineInfo struct {
	Pos  int64 // Position from the start of the file
	Size int64 // Size of this line
}

// rotateLogFile rotates the log file based on the retention policy.
// It writes to the result and returns an error if any.
//
// The file is rotated by reading the file backward line-by-line
// and stop once error occurs or found a line that should not be kept.
//
// Any invalid lines will be skipped and not included in the result.
//
// If the file does not need to be rotated, it returns nil, nil.
func rotateLogFile(file supportRotate, config *Retention, result *RotateResult) (rotated bool, err error) {
	if config.KeepSize > 0 {
		rotated, err = rotateLogFileBySize(file, config, result)
	} else {
		rotated, err = rotateLogFileByPolicy(file, config, result)
	}

	_, ferr := file.Seek(0, io.SeekEnd)
	err = errors.Join(err, ferr)
	return
}

func rotateLogFileByPolicy(file supportRotate, config *Retention, result *RotateResult) (rotated bool, err error) {
	var shouldStop func() bool
	t := mockable.TimeNow()

	switch {
	case config.Last > 0:
		shouldStop = func() bool { return result.NumLinesKeep-result.NumLinesInvalid == int(config.Last) }
		// not needed to parse time for last N lines
	case config.Days > 0:
		cutoff := mockable.TimeNow().AddDate(0, 0, -int(config.Days)+1)
		shouldStop = func() bool { return t.Before(cutoff) }
	default:
		return false, nil // should not happen
	}

	stat, err := file.Stat()
	if err != nil {
		return false, err
	}
	fileSize := stat.Size()

	// nothing to rotate, return the nothing
	if fileSize == 0 {
		return false, nil
	}

	s := NewBackScanner(file, fileSize, defaultChunkSize)
	defer s.Release()
	result.OriginalSize = fileSize

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
		return false, s.Err()
	}

	// nothing to keep, truncate to empty
	if len(linesToKeep) == 0 {
		return true, file.Truncate(0)
	}

	// nothing to rotate, return nothing
	if result.NumBytesKeep == result.OriginalSize {
		return false, nil
	}

	// Read each line and write it to the beginning of the file
	writePos := int64(0)
	// in reverse order to keep the order of the lines (from old to new)
	for i := len(linesToKeep) - 1; i >= 0; i-- {
		line := linesToKeep[i]
		n := line.Size

		if err := fileContentMove(file, line.Pos, writePos, int(n)); err != nil {
			return false, err
		}
		writePos += n
	}

	if err := file.Truncate(writePos); err != nil {
		return false, err
	}

	return true, nil
}

// fileContentMove moves the content of the file from the source position to the destination position.
//
// this is only used for moving from the back to the front of the file.
func fileContentMove(file supportRotate, srcPos, dstPos int64, size int) error {
	buf := sizedPool.GetSized(size)
	defer sizedPool.Put(buf)

	// Read the line from its original position
	nRead, err := file.ReadAt(buf, srcPos)
	if err != nil {
		return err
	}
	if nRead != size {
		return fmt.Errorf("%w, reading %d bytes, only %d read", io.ErrShortBuffer, size, nRead)
	}

	// Write it to the new position
	nWritten, err := file.WriteAt(buf, dstPos)
	if err != nil {
		return err
	}
	if nWritten != size {
		return fmt.Errorf("%w, writing %d bytes, only %d written", io.ErrShortWrite, size, nWritten)
	}

	return nil
}

// rotateLogFileBySize rotates the log file by size.
// It returns the result of the rotation and an error if any.
//
// The file is not being read, it just truncate the file to the new size.
//
// Invalid lines will not be detected and included in the result.
func rotateLogFileBySize(file supportRotate, config *Retention, result *RotateResult) (rotated bool, err error) {
	stat, err := file.Stat()
	if err != nil {
		return false, err
	}
	fileSize := stat.Size()

	result.OriginalSize = fileSize

	keepSize := int64(config.KeepSize)
	if keepSize >= fileSize {
		result.NumBytesKeep = fileSize
		return false, nil
	}
	result.NumBytesKeep = keepSize

	err = file.Truncate(keepSize)
	if err != nil {
		return false, err
	}

	return true, nil
}

// ParseLogTime parses the time from the log line.
// It returns the time if the time is found and valid in the log line,
// otherwise it returns zero time.
func ParseLogTime(line []byte) (t time.Time) {
	if len(line) == 0 {
		return t
	}

	if timeStr := ExtractTime(line); timeStr != nil {
		t, _ = time.Parse(LogTimeFormat, string(timeStr)) // ignore error
		return t
	}
	return t
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
			jsonStart := i + len(`"time":"`)
			jsonEnd := i + len(`"time":"`) + len(LogTimeFormat)
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

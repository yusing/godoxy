package serialization

import (
	"bytes"
	"io"
)

type SubstituteEnvReader struct {
	reader io.Reader
	buf    []byte // buffered data with substitutions applied
	err    error  // sticky error
}

func NewSubstituteEnvReader(reader io.Reader) *SubstituteEnvReader {
	return &SubstituteEnvReader{reader: reader}
}

const peekSize = 4096
const maxVarNameLength = 256

func (r *SubstituteEnvReader) Read(p []byte) (n int, err error) {
	// Return buffered data first
	if len(r.buf) > 0 {
		n = copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}

	// Return sticky error if we have one
	if r.err != nil {
		return 0, r.err
	}

	var buf [2 * peekSize]byte

	// Read a chunk from the underlying reader
	chunk, more := buf[:peekSize], buf[peekSize:]
	nRead, readErr := r.reader.Read(chunk)
	if nRead == 0 {
		if readErr != nil {
			return 0, readErr
		}
		return 0, io.EOF
	}
	chunk = chunk[:nRead]

	// Check if there's a potential incomplete pattern at the end
	// Pattern: ${VAR_NAME}
	// We need to check if chunk ends with a partial pattern like "$", "${", "${VAR", etc.
	incompleteStart := findIncompletePatternStart(chunk)

	if incompleteStart >= 0 && readErr == nil {
		// There might be an incomplete pattern, read more to complete it
		incomplete := chunk[incompleteStart:]
		chunk = chunk[:incompleteStart]

		// Keep reading until we complete the pattern or hit EOF/error
		for {
			// Limit how much we buffer to prevent memory exhaustion
			if len(incomplete) > maxVarNameLength+3 { // ${} + var name
				// Pattern too long to be valid, give up and process as-is
				chunk = append(chunk, incomplete...)
				break
			}
			nMore, moreErr := r.reader.Read(more)
			if nMore > 0 {
				incomplete = append(incomplete, more[:nMore]...)
				// Check if pattern is now complete
				if idx := bytes.IndexByte(incomplete, '}'); idx >= 0 {
					// Pattern complete, append the rest back to chunk
					chunk = append(chunk, incomplete...)
					break
				}
			}
			if moreErr != nil {
				// No more data, append whatever we have
				chunk = append(chunk, incomplete...)
				readErr = moreErr
				break
			}
		}
	}

	substituted, subErr := substituteEnv(chunk)
	if subErr != nil {
		r.err = subErr
		return 0, subErr
	}

	n = copy(p, substituted)
	if n < len(substituted) {
		// Buffer the rest
		r.buf = substituted[n:]
	}

	// Store sticky error for next read
	if readErr != nil && readErr != io.EOF {
		r.err = readErr
	} else {
		if readErr == io.EOF && n > 0 {
			return n, nil
		}
		if readErr == io.EOF {
			return n, io.EOF
		}
	}

	return n, nil
}

// findIncompletePatternStart returns the index where an incomplete ${...} pattern starts,
// or -1 if there's no incomplete pattern at the end.
func findIncompletePatternStart(data []byte) int {
	// Look for '$' near the end that might be start of ${VAR}
	// Maximum var name we reasonably expect + "${}" = ~256 chars
	searchStart := max(0, len(data)-maxVarNameLength)

	for i := len(data) - 1; i >= searchStart; i-- {
		if data[i] == '$' {
			// Check if this is a complete pattern or incomplete
			if i+1 >= len(data) {
				// Just "$" at end
				return i
			}
			if data[i+1] == '{' {
				// Check if there's anything after "${"
				if i+2 >= len(data) {
					// Just "${" at end
					return i
				}
				// Check if pattern is complete by looking for '}'
				for j := i + 2; j < len(data); j++ {
					if data[j] == '}' {
						// This pattern is complete, continue searching for another
						break
					}
					if j == len(data)-1 {
						// Reached end without finding '}', incomplete pattern
						return i
					}
				}
			}
		}
	}
	return -1
}

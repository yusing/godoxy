package strutils

import "strings"

// IsValidFilename checks if a filename is safe and doesn't contain path traversal attempts
// Returns true if the filename is valid, false otherwise
func IsValidFilename(filename string) bool {
	return !strings.Contains(filename, "/") &&
		!strings.Contains(filename, "\\") &&
		!strings.Contains(filename, "..")
}

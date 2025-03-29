package strutils

import (
	"path"
	"strings"
)

// SanitizeURI sanitizes a URI reference to ensure it is safe
// It disallows URLs beginning with // or /\ as absolute URLs,
// cleans the URL path to remove any .. or . path elements,
// and ensures the URL starts with a / if it doesn't already
func SanitizeURI(uri string) string {
	if uri == "" {
		return "/"
	}
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return uri
	}
	if uri[0] != '/' {
		uri = "/" + uri
	}
	if len(uri) > 1 && uri[0] == '/' && uri[1] != '/' && uri[1] != '\\' {
		return path.Clean(uri)
	}
	return "/"
}

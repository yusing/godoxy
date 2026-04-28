package rules

import (
	"net/http"
	"strings"

	"github.com/puzpuzpuz/xsync/v4"
)

var namedHandlers = xsync.NewMap[string, http.Handler]()

func normalizeHandlerName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// RegisterHandler registers a handler with the given name.
// Returns true if the handler was registered, false if the name was empty,
// the handler was nil or a handler with the same name was already registered.
func RegisterHandler(name string, handler http.Handler) bool {
	name = normalizeHandlerName(name)
	if name == "" || handler == nil {
		return false
	}
	_, loaded := namedHandlers.LoadOrStore(name, handler)
	return !loaded
}

func GetHandler(name string) (http.Handler, bool) {
	return namedHandlers.Load(normalizeHandlerName(name))
}

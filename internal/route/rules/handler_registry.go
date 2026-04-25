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

func RegisterHandler(name string, handler http.Handler) {
	name = normalizeHandlerName(name)
	if name == "" || handler == nil {
		return
	}
	namedHandlers.Store(name, handler)
}

func GetHandler(name string) (http.Handler, bool) {
	return namedHandlers.Load(normalizeHandlerName(name))
}

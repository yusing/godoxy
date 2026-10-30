package middleware

import (
	"net/http"

	"github.com/yusing/godoxy/internal/logging/accesslog"
	httpevents "github.com/yusing/goutils/events/http"
)

// blocked emits a "blocked" event and records the denial for the access log.
func blocked(r *http.Request, source, reason string) {
	httpevents.Blocked(r, source, reason)
	accesslog.RecordDenied(r, source, reason)
}

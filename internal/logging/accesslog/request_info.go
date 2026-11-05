package accesslog

import (
	"context"
	"net/http"
)

// requestInfo carries access log annotations that are only known while the
// request is being handled.
type requestInfo struct {
	// PeerIP is the connection's address before real IP middlewares
	// rewrote RemoteAddr.
	PeerIP string
	// DeniedBy names the component that denied the request, e.g.
	// "CIDRWhitelist"; empty if none did.
	DeniedBy string
	// DenyReason explains the denial.
	DenyReason string
}

type requestInfoKey struct{}

// WithRequestInfo attaches an annotation slot to r and records its peer IP.
// Call it before any middleware rewrites RemoteAddr. It returns r unchanged
// if a slot is already attached.
//
// The slot is written and read on the request's serving goroutine.
func WithRequestInfo(r *http.Request) *http.Request {
	if getRequestInfo(r) != nil {
		return r
	}
	info := &requestInfo{PeerIP: clientIP(r)}
	return r.WithContext(context.WithValue(r.Context(), requestInfoKey{}, info))
}

// RecordDenied records that source denied r because of reason. The first
// denial wins. It is a no-op when r has no slot, i.e. when no access log will
// write the request.
func RecordDenied(r *http.Request, source, reason string) {
	info := getRequestInfo(r)
	if info == nil || info.DeniedBy != "" {
		return
	}
	info.DeniedBy = source
	info.DenyReason = reason
}

func getRequestInfo(r *http.Request) *requestInfo {
	info, _ := r.Context().Value(requestInfoKey{}).(*requestInfo)
	return info
}

// DenyReason returns the recorded denial reason, or an empty string if none was recorded.
func DenyReason(r *http.Request) string {
	if info := getRequestInfo(r); info != nil {
		return info.DenyReason
	}
	return ""
}

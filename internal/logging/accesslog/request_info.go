package accesslog

import (
	"context"
	"net/http"
)

// RequestInfo carries access log annotations that are only known while the
// request is being handled.
type RequestInfo struct {
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
	info := &RequestInfo{PeerIP: clientIP(r)}
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

// RequestInfoOf returns a copy of r's annotations, false if r has no slot.
func RequestInfoOf(r *http.Request) (RequestInfo, bool) {
	info := getRequestInfo(r)
	if info == nil {
		return RequestInfo{}, false
	}
	return *info, true
}

func getRequestInfo(r *http.Request) *RequestInfo {
	info, _ := r.Context().Value(requestInfoKey{}).(*RequestInfo)
	return info
}

// peerIPIfRewritten returns the recorded peer IP when it differs from the
// logged client IP, i.e. when a real IP middleware rewrote RemoteAddr.
func (info *RequestInfo) peerIPIfRewritten(req *http.Request) string {
	if info == nil || info.PeerIP == "" || info.PeerIP == clientIP(req) {
		return ""
	}
	return info.PeerIP
}

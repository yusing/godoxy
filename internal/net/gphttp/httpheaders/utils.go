package httpheaders

import (
	"net/http"
	"net/textproto"
	"strings"

	"golang.org/x/net/http/httpguts"
)

const (
	HeaderXForwardedMethod = "X-Forwarded-Method"
	HeaderXForwardedFor    = "X-Forwarded-For"
	HeaderXForwardedProto  = "X-Forwarded-Proto"
	HeaderXForwardedHost   = "X-Forwarded-Host"
	HeaderXForwardedPort   = "X-Forwarded-Port"
	HeaderXForwardedURI    = "X-Forwarded-Uri"
	HeaderXRealIP          = "X-Real-IP"

	HeaderContentType   = "Content-Type"
	HeaderContentLength = "Content-Length"

	HeaderGoDoxyCheckRedirect = "X-Godoxy-Check-Redirect"
)

// Hop-by-hop headers. These are removed when sent to the backend.
// As of RFC 7230, hop-by-hop headers are required to appear in the
// Connection header field. These are the headers defined by the
// obsoleted RFC 2616 (section 13.5.1) and are used for backward
// compatibility.
var hopHeaders = []string{
	"Connection",
	"Proxy-Connection", // non-standard but still sent by libcurl and rejected by e.g. google
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",      // canonicalized version of "TE"
	"Trailer", // not Trailers per URL above; https://www.rfc-editor.org/errata_search.php?eid=4522
	"Transfer-Encoding",
	"Upgrade",
}

func UpgradeType(h http.Header) string {
	if !httpguts.HeaderValuesContainsToken(h["Connection"], "Upgrade") {
		return ""
	}
	return h.Get("Upgrade")
}

// RemoveHopByHopHeaders removes hop-by-hop headers.
func RemoveHopByHopHeaders(h http.Header) {
	// RFC 7230, section 6.1: Remove headers listed in the "Connection" header.
	for _, f := range h["Connection"] {
		for sf := range strings.SplitSeq(f, ",") {
			if sf = textproto.TrimString(sf); sf != "" {
				h.Del(sf)
			}
		}
	}
	// RFC 2616, section 13.5.1: Remove a set of known hop-by-hop headers.
	// This behavior is superseded by the RFC 7230 Connection header, but
	// preserve it for backwards compatibility.
	for _, f := range hopHeaders {
		h.Del(f)
	}
}

func RemoveHop(h http.Header) {
	reqUpType := UpgradeType(h)
	RemoveHopByHopHeaders(h)

	if reqUpType != "" {
		h.Set("Connection", "Upgrade")
		h.Set("Upgrade", reqUpType)
	} else {
		h.Del("Connection")
	}
}

func RemoveServiceHeaders(h http.Header) {
	h.Del("X-Powered-By")
	h.Del("Server")
}

func CopyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func FilterHeaders(h http.Header, allowed []string) http.Header {
	if len(allowed) == 0 {
		return h
	}

	filtered := make(http.Header)

	for i, header := range allowed {
		values := h.Values(header)
		if len(values) == 0 {
			continue
		}
		filtered[http.CanonicalHeaderKey(allowed[i])] = append([]string(nil), values...)
	}

	return filtered
}

func HeaderToMap(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0] // Take the first value
		}
	}
	return result
}

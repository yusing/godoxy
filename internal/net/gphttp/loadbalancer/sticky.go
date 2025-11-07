package loadbalancer

import (
	"encoding/hex"
	"net/http"
	"time"
	"unsafe"

	"github.com/bytedance/gopkg/util/xxhash3"
	"github.com/yusing/godoxy/internal/types"
)

func hashServerKey(key string) string {
	h := xxhash3.HashString(key)
	as8bytes := *(*[8]byte)(unsafe.Pointer(&h))
	return hex.EncodeToString(as8bytes[:])
}

// getStickyServer extracts the sticky session cookie and returns the corresponding server
func getStickyServer(r *http.Request, srvs []types.LoadBalancerServer) types.LoadBalancerServer {
	cookie, err := r.Cookie("godoxy_lb_sticky")
	if err != nil {
		return nil
	}

	serverKeyHash := cookie.Value
	for _, srv := range srvs {
		if hashServerKey(srv.Key()) == serverKeyHash {
			return srv
		}
	}
	return nil
}

// setStickyCookie sets a cookie to maintain sticky session with a specific server
func setStickyCookie(rw http.ResponseWriter, r *http.Request, srv types.LoadBalancerServer, maxAge time.Duration) {
	http.SetCookie(rw, &http.Cookie{
		Name:     "godoxy_lb_sticky",
		Value:    hashServerKey(srv.Key()),
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		SameSite: http.SameSiteLaxMode,
		HttpOnly: true,
		Secure:   isSecure(r),
	})
}

func isSecure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

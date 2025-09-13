package middleware

import (
	"net"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/puzpuzpuz/xsync/v4"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/serialization"
)

type (
	cidrWhitelist struct {
		CIDRWhitelistOpts
		cachedAddr *xsync.Map[string, bool] // cache for trusted IPs
	}
	CIDRWhitelistOpts struct {
		Allow      []*nettypes.CIDR `validate:"min=1"`
		StatusCode int              `json:"status_code" aliases:"status" validate:"omitempty,status_code"`
		Message    string
	}
)

var (
	CIDRWhiteList         = NewMiddleware[cidrWhitelist]()
	cidrWhitelistDefaults = CIDRWhitelistOpts{
		Allow:      []*nettypes.CIDR{},
		StatusCode: http.StatusForbidden,
		Message:    "IP not allowed",
	}
)

func init() {
	serialization.MustRegisterValidation("status_code", func(fl validator.FieldLevel) bool {
		statusCode := fl.Field().Int()
		return gphttp.IsStatusCodeValid(int(statusCode))
	})
}

// setup implements MiddlewareWithSetup.
func (wl *cidrWhitelist) setup() {
	wl.CIDRWhitelistOpts = cidrWhitelistDefaults
	wl.cachedAddr = xsync.NewMap[string, bool](xsync.WithPresize(100))
}

// before implements RequestModifier.
func (wl *cidrWhitelist) before(w http.ResponseWriter, r *http.Request) bool {
	return wl.checkIP(w, r)
}

// checkIP checks if the IP address is allowed.
func (wl *cidrWhitelist) checkIP(w http.ResponseWriter, r *http.Request) bool {
	var allow, ok bool
	if allow, ok = wl.cachedAddr.Load(r.RemoteAddr); !ok {
		ipStr, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ipStr = r.RemoteAddr
		}
		ip := net.ParseIP(ipStr)
		for _, cidr := range wl.Allow {
			if cidr.Contains(ip) {
				wl.cachedAddr.Store(r.RemoteAddr, true)
				allow = true
				break
			}
		}
		if !allow {
			wl.cachedAddr.Store(r.RemoteAddr, false)
		}
	}
	if !allow {
		http.Error(w, wl.Message, wl.StatusCode)
		return false
	}

	return true
}

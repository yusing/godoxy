package middleware

import (
	"net"
	"net/http"

	"github.com/go-playground/validator/v10"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/serialization"
	httpevents "github.com/yusing/goutils/events/http"
	httputils "github.com/yusing/goutils/http"
)

type (
	cidrWhitelist struct {
		CIDRWhitelistOpts
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
		return httputils.IsStatusCodeValid(int(statusCode))
	})
}

// setup implements MiddlewareWithSetup.
func (wl *cidrWhitelist) setup() {
	wl.CIDRWhitelistOpts = cidrWhitelistDefaults
}

// before implements RequestModifier.
func (wl *cidrWhitelist) before(w http.ResponseWriter, r *http.Request) bool {
	return wl.checkIP(w, r)
}

// checkIP checks if the IP address is allowed.
func (wl *cidrWhitelist) checkIP(w http.ResponseWriter, r *http.Request) bool {
	var allow bool
	ipStr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		allow = ipInCIDRs(ipStr, wl.Allow)
	}
	if !allow {
		defer httpevents.Blocked(r, "CIDRWhitelist", "IP not allowed")
		http.Error(w, wl.Message, wl.StatusCode)
		return false
	}
	return true
}

func ipInCIDRs(ipStr string, cidrs []*nettypes.CIDR) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

package middleware

import (
	"net"
	"net/http"

	"github.com/go-playground/validator/v10"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils"
	F "github.com/yusing/go-proxy/internal/utils/functional"
)

type (
	cidrWhitelist struct {
		CIDRWhitelistOpts
		Tracer
		cachedAddr F.Map[string, bool] // cache for trusted IPs
	}
	CIDRWhitelistOpts struct {
		Allow      []*types.CIDR `validate:"min=1"`
		StatusCode int           `json:"status_code" aliases:"status" validate:"omitempty,status_code"`
		Message    string
	}
)

var (
	CIDRWhiteList         = NewMiddleware[cidrWhitelist]()
	cidrWhitelistDefaults = CIDRWhitelistOpts{
		Allow:      []*types.CIDR{},
		StatusCode: http.StatusForbidden,
		Message:    "IP not allowed",
	}
)

func init() {
	utils.MustRegisterValidation("status_code", func(fl validator.FieldLevel) bool {
		statusCode := fl.Field().Int()
		return gphttp.IsStatusCodeValid(int(statusCode))
	})
}

// setup implements MiddlewareWithSetup.
func (wl *cidrWhitelist) setup() {
	wl.CIDRWhitelistOpts = cidrWhitelistDefaults
	wl.cachedAddr = F.NewMapOf[string, bool]()
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
		for _, cidr := range wl.CIDRWhitelistOpts.Allow {
			if cidr.Contains(ip) {
				wl.cachedAddr.Store(r.RemoteAddr, true)
				allow = true
				wl.AddTracef("client %s is allowed", ipStr).With("allowed CIDR", cidr)
				break
			}
		}
		if !allow {
			wl.cachedAddr.Store(r.RemoteAddr, false)
			wl.AddTracef("client %s is forbidden", ipStr).With("allowed CIDRs", wl.CIDRWhitelistOpts.Allow)
		}
	}
	if !allow {
		http.Error(w, wl.Message, wl.StatusCode)
		return false
	}

	return true
}

package healthcheck

import (
	"net/url"

	"github.com/yusing/godoxy/internal/types"
)

func invalidTargetURL(u *url.URL) (types.HealthCheckResult, bool) {
	switch {
	case u == nil:
		return types.HealthCheckResult{Detail: "no url specified"}, true
	case u.Host == "":
		return types.HealthCheckResult{Detail: "no host specified"}, true
	default:
		return types.HealthCheckResult{}, false
	}
}

package healthcheck

import (
	"net/url"

	"github.com/yusing/godoxy/internal/health"
)

func invalidTargetURL(u *url.URL) (health.HealthCheckResult, bool) {
	switch {
	case u == nil:
		return health.HealthCheckResult{Detail: "no url specified"}, true
	case u.Host == "":
		return health.HealthCheckResult{Detail: "no host specified"}, true
	default:
		return health.HealthCheckResult{}, false
	}
}

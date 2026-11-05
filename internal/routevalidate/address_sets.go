package routevalidate

import (
	"context"
	"errors"
	"fmt"

	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/route"
)

// ValidateAddressSources covers inline rules, rule files, access logs and
// middleware, including composed chains. During preparation references use the
// candidate's sets; standalone validation uses the active definitions.
func ValidateAddressSources(ctx context.Context, r *route.Route) error {
	errs := []error{validateRules(r)}
	check := config.AddressSetCheck(ctx)
	for name, value := range map[string]any{"rules": r.Rules, "access_log": r.AccessLog} {
		if err := nettypes.ValidateAddressSetReferences(value, check); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	if err := middleware.ValidateAddressSetMap(r.Middlewares, check); err != nil {
		errs = append(errs, fmt.Errorf("middlewares: %w", err))
	}
	return errors.Join(errs...)
}

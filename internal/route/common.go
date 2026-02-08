package route

import (
	"context"
	"fmt"

	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/types"
)

// checkExists checks if the route already exists in the entrypoint.
//
// Context must be passed from the parent task that carries the entrypoint value.
func checkExists(ctx context.Context, r types.Route) error {
	if r.UseLoadBalance() { // skip checking for load balanced routes
		return nil
	}
	var (
		existing types.Route
		ok       bool
	)
	switch r := r.(type) {
	case types.HTTPRoute:
		existing, ok = entrypoint.FromCtx(ctx).HTTPRoutes().Get(r.Key())
	case types.StreamRoute:
		existing, ok = entrypoint.FromCtx(ctx).StreamRoutes().Get(r.Key())
	}
	if ok {
		return fmt.Errorf("route already exists: from provider %s and %s", existing.ProviderName(), r.ProviderName())
	}
	return nil
}

package middleware

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/godoxy/internal/serialization"
	strutils "github.com/yusing/goutils/strings"
)

type EntrypointRouteOverlay struct {
	Middleware          *Middleware
	ConsumedBypass      map[string]struct{}
	ConsumedMiddlewares map[string]struct{}
}

type bypassOnlyField struct {
	Bypass Bypass `json:"bypass"`
}

var ErrNoEntrypointRouteOverlay = errors.New("no entrypoint route overlay")

// BuildEntrypointRouteOverlay promotes route-level bypass rules into a copy of the entrypoint middleware
// chain. For each route middleware entry in routeMiddlewares that sets "bypass", it finds the entrypoint
// definition with the same "use" name (case-insensitive, snake-agnostic) and appends those rules after
// qualifying them with the route (each rule becomes "route <routeName> & <original>").
//
// name is the logical chain name passed to [BuildMiddlewareFromChainRaw].
//
// It returns [ErrNoEntrypointRouteOverlay] when entrypointDefs or routeMiddlewares is empty, or when no
// route bypass was merged into any entrypoint definition. On success, ConsumedBypass lists normalized
// middleware names whose bypass was applied; ConsumedMiddlewares lists names whose route options contained
// only "bypass", so downstream handling can treat those overlay-only route entries as fully satisfied.
// Route middleware entries with additional options still run at route scope after promotion.
//
// Errors wrap parse/merge failures for bypass values or route qualification.
func BuildEntrypointRouteOverlay(
	name string,
	entrypointDefs []map[string]any,
	routeName string,
	routeMiddlewares map[string]OptionsRaw,
) (*EntrypointRouteOverlay, error) {
	if len(entrypointDefs) == 0 || len(routeMiddlewares) == 0 {
		return nil, ErrNoEntrypointRouteOverlay
	}

	effectiveDefs := cloneMiddlewareDefs(entrypointDefs)
	var consumedBypass map[string]struct{}
	var consumedMiddlewares map[string]struct{}
	promotedAny := false

	for routeMiddlewareName, routeOpts := range routeMiddlewares {
		promotedBypass, ok, err := buildPromotedRouteBypass(routeName, routeMiddlewareName, routeOpts)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		matched, err := mergePromotedBypassIntoEffectiveDefs(effectiveDefs, routeMiddlewareName, promotedBypass)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}

		promotedAny = true
		consumedBypass, consumedMiddlewares = recordPromotedRouteOverlayConsumption(
			consumedBypass,
			consumedMiddlewares,
			routeMiddlewareName,
			routeOpts,
		)
	}

	if !promotedAny {
		return nil, ErrNoEntrypointRouteOverlay
	}

	mid, err := BuildMiddlewareFromChainRaw(name, effectiveDefs)
	if err != nil {
		return nil, err
	}
	return &EntrypointRouteOverlay{
		Middleware:          mid,
		ConsumedBypass:      consumedBypass,
		ConsumedMiddlewares: consumedMiddlewares,
	}, nil
}

func buildPromotedRouteBypass(routeName, routeMiddlewareName string, routeOpts OptionsRaw) (Bypass, bool, error) {
	routeBypass, ok, err := parseBypassValue(routeOpts["bypass"])
	if err != nil {
		return nil, false, fmt.Errorf("route middleware %q bypass: %w", routeMiddlewareName, err)
	}
	if !ok || len(routeBypass) == 0 {
		return nil, false, nil
	}

	promotedBypass, err := qualifyBypassWithRoute(routeName, routeBypass)
	if err != nil {
		return nil, false, fmt.Errorf("route middleware %q bypass promotion: %w", routeMiddlewareName, err)
	}
	return promotedBypass, true, nil
}

func mergePromotedBypassIntoEffectiveDefs(effectiveDefs []map[string]any, routeMiddlewareName string, promotedBypass Bypass) (bool, error) {
	normalizedRouteMiddlewareName := strutils.ToLowerNoSnake(routeMiddlewareName)
	matched := false
	for i, def := range effectiveDefs {
		use, _ := def["use"].(string)
		if strutils.ToLowerNoSnake(use) != normalizedRouteMiddlewareName {
			continue
		}

		mergedBypass, err := appendBypassValue(def["bypass"], promotedBypass)
		if err != nil {
			return false, fmt.Errorf("entrypoint middleware %q bypass merge: %w", use, err)
		}

		clonedDef := maps.Clone(def)
		clonedDef["bypass"] = mergedBypass
		effectiveDefs[i] = clonedDef
		matched = true
	}
	return matched, nil
}

func recordPromotedRouteOverlayConsumption(
	consumedBypass map[string]struct{},
	consumedMiddlewares map[string]struct{},
	routeMiddlewareName string,
	routeOpts OptionsRaw,
) (map[string]struct{}, map[string]struct{}) {
	normalizedName := strutils.ToLowerNoSnake(routeMiddlewareName)
	if consumedBypass == nil {
		consumedBypass = make(map[string]struct{})
	}
	consumedBypass[normalizedName] = struct{}{}

	if !isBypassOnlyOptions(routeOpts) {
		return consumedBypass, consumedMiddlewares
	}
	if consumedMiddlewares == nil {
		consumedMiddlewares = make(map[string]struct{})
	}
	consumedMiddlewares[normalizedName] = struct{}{}
	return consumedBypass, consumedMiddlewares
}

func cloneMiddlewareDefs(defs []map[string]any) []map[string]any {
	cloned := make([]map[string]any, len(defs))
	for i, def := range defs {
		// Shallow clone is intentional: overlay promotion only replaces the top-level
		// bypass field and leaves nested option values untouched.
		cloned[i] = maps.Clone(def)
	}
	return cloned
}

func appendBypassValue(existing any, promoted Bypass) (Bypass, error) {
	current, ok, err := parseBypassValue(existing)
	if err != nil {
		return nil, err
	}
	if !ok {
		return slices.Clone(promoted), nil
	}
	return append(slices.Clone(current), promoted...), nil
}

func parseBypassValue(raw any) (Bypass, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	var dst bypassOnlyField
	if err := serialization.MapUnmarshalValidate(map[string]any{"bypass": raw}, &dst); err != nil {
		return nil, true, err
	}
	return dst.Bypass, true, nil
}

func qualifyBypassWithRoute(routeName string, bypass Bypass) (Bypass, error) {
	qualified := make(Bypass, len(bypass))
	for i, rule := range bypass {
		var routeQualified rules.RuleOn
		if err := routeQualified.Parse(fmt.Sprintf("route %s & %s", routeName, rule.String())); err != nil {
			return nil, err
		}
		qualified[i] = routeQualified
	}
	return qualified, nil
}

func isBypassOnlyOptions(opts OptionsRaw) bool {
	if len(opts) == 0 {
		return false
	}
	for key := range opts {
		if strutils.ToLowerNoSnake(key) != "bypass" {
			return false
		}
	}
	return true
}

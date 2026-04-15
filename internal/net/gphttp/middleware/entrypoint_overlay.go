package middleware

import (
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

// BuildEntrypointRouteOverlay promotes route-level bypass rules into a copy of the entrypoint middleware
// chain. For each route middleware entry in routeMiddlewares that sets "bypass", it finds the entrypoint
// definition with the same "use" name (case-insensitive, snake-agnostic) and appends those rules after
// qualifying them with the route (each rule becomes "route <routeName> & <original>").
//
// name is the logical chain name passed to [BuildMiddlewareFromChainRaw].
//
// It returns (nil, nil) when entrypointDefs or routeMiddlewares is empty, or when no route bypass was
// merged into any entrypoint definition. On success, ConsumedBypass lists normalized middleware names
// whose bypass was applied; ConsumedMiddlewares lists names whose route options contained only "bypass",
// so downstream handling can treat those overlay-only route entries as fully satisfied. Route middleware
// entries with additional options still run at route scope after promotion.
//
// Errors wrap parse/merge failures for bypass values or route qualification.
func BuildEntrypointRouteOverlay(
	name string,
	entrypointDefs []map[string]any,
	routeName string,
	routeMiddlewares map[string]OptionsRaw,
) (*EntrypointRouteOverlay, error) {
	if len(entrypointDefs) == 0 || len(routeMiddlewares) == 0 {
		return nil, nil
	}

	effectiveDefs := cloneMiddlewareDefs(entrypointDefs)
	var consumedBypass map[string]struct{}
	var consumedMiddlewares map[string]struct{}
	promotedAny := false

	for routeMiddlewareName, routeOpts := range routeMiddlewares {
		routeBypass, ok, err := parseBypassValue(routeOpts["bypass"])
		if err != nil {
			return nil, fmt.Errorf("route middleware %q bypass: %w", routeMiddlewareName, err)
		}
		if !ok || len(routeBypass) == 0 {
			continue
		}

		promotedBypass, err := qualifyBypassWithRoute(routeName, routeBypass)
		if err != nil {
			return nil, fmt.Errorf("route middleware %q bypass promotion: %w", routeMiddlewareName, err)
		}

		matched := false
		for i, def := range effectiveDefs {
			use, _ := def["use"].(string)
			if strutils.ToLowerNoSnake(use) != strutils.ToLowerNoSnake(routeMiddlewareName) {
				continue
			}

			mergedBypass, err := appendBypassValue(def["bypass"], promotedBypass)
			if err != nil {
				return nil, fmt.Errorf("entrypoint middleware %q bypass merge: %w", use, err)
			}

			def = maps.Clone(def)
			def["bypass"] = mergedBypass
			effectiveDefs[i] = def
			matched = true
		}
		if !matched {
			continue
		}

		promotedAny = true
		if consumedBypass == nil {
			consumedBypass = make(map[string]struct{})
		}
		normalizedName := strutils.ToLowerNoSnake(routeMiddlewareName)
		consumedBypass[normalizedName] = struct{}{}
		if isBypassOnlyOptions(routeOpts) {
			if consumedMiddlewares == nil {
				consumedMiddlewares = make(map[string]struct{})
			}
			consumedMiddlewares[normalizedName] = struct{}{}
		}
	}

	if !promotedAny {
		return nil, nil
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

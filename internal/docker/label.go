package docker

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
)

var ErrInvalidLabel = errors.New("invalid label")

const nsProxyDot = NSProxy + "."

func ParseLabels(labels map[string]string, aliases ...string) (types.LabelMap, error) {
	nestedMap := make(types.LabelMap)
	errs := gperr.NewBuilder("labels error")

	ExpandWildcard(labels, aliases...)

	keys := slices.SortedFunc(maps.Keys(labels), compareLabelKeys)

labelLoop:
	for _, lbl := range keys {
		value := labels[lbl]
		parts := strings.Split(lbl, ".")
		if parts[0] != NSProxy {
			continue
		}
		if len(parts) == 1 {
			errs.AddSubject(ErrInvalidLabel, lbl)
			continue
		}
		parts = parts[1:]
		currentMap := nestedMap

		for i, k := range parts {
			if i == len(parts)-1 {
				if existing, ok := currentMap[k].(types.LabelMap); ok {
					objectValue, isObject := parseLabelObject(value)
					if !isObject {
						errs.AddSubject(fmt.Errorf("expect mapping, got %T", value), lbl)
						continue labelLoop
					}
					if err := mergeLabelMaps(existing, objectValue); err != nil {
						errs.AddSubject(err, lbl)
						continue labelLoop
					}
					continue labelLoop
				}

				// Last element, set the value.
				currentMap[k] = value
				continue labelLoop
			}

			// If the key doesn't exist, create a new map.
			if _, exists := currentMap[k]; !exists {
				nextMap := make(types.LabelMap)
				currentMap[k] = nextMap
				currentMap = nextMap
				continue
			}

			// Move deeper into the nested map.
			switch next := currentMap[k].(type) {
			case types.LabelMap:
				currentMap = next
			case string:
				objectValue, isObject := parseLabelObject(next)
				if !isObject {
					errs.AddSubject(fmt.Errorf("expect mapping, got %T", currentMap[k]), lbl)
					continue labelLoop
				}
				currentMap[k] = objectValue
				currentMap = objectValue
			default:
				errs.AddSubject(fmt.Errorf("expect mapping, got %T", currentMap[k]), lbl)
				continue labelLoop
			}
		}
	}

	return nestedMap, errs.Error()
}

func parseLabelObject(value string) (types.LabelMap, bool) {
	if value == "" {
		return make(types.LabelMap), true
	}

	objectValue := make(types.LabelMap)
	if err := yaml.Unmarshal([]byte(strings.ReplaceAll(value, "\t", "  ")), &objectValue); err != nil {
		return nil, false
	}
	return objectValue, true
}

func mergeLabelMaps(dst, src types.LabelMap) error {
	for key, srcValue := range src {
		existingValue, exists := dst[key]
		if !exists {
			dst[key] = srcValue
			continue
		}

		existingMap, existingIsMap := existingValue.(types.LabelMap)
		srcMap, srcIsMap := srcValue.(types.LabelMap)
		if existingIsMap && srcIsMap {
			if err := mergeLabelMaps(existingMap, srcMap); err != nil {
				return err
			}
			continue
		}
		if existingIsMap {
			return fmt.Errorf("expect mapping, got %T", srcValue)
		}
		if srcIsMap {
			return fmt.Errorf("expect scalar, got %T", srcValue)
		}
	}
	return nil
}

func compareLabelKeys(a, b string) int {
	if parts := cmp.Compare(strings.Count(a, "."), strings.Count(b, ".")); parts != 0 {
		return parts
	}
	return cmp.Compare(a, b)
}

func ExpandWildcard(labels map[string]string, aliases ...string) {
	aliasSet := make(map[string]int, len(aliases))
	for i, alias := range aliases {
		aliasSet[alias] = i
	}

	wildcardLabels := make(map[string]string)

	// First pass: collect wildcards and discover aliases
	for lbl, value := range labels {
		if !strings.HasPrefix(lbl, nsProxyDot) {
			continue
		}
		// lbl is "proxy.X..." where X is alias or wildcard
		rest := lbl[len(nsProxyDot):] // "X..." or "X.suffix"
		alias, suffix, _ := strings.Cut(rest, ".")
		if alias == WildcardAlias {
			delete(labels, lbl)
			if suffix == "" || strings.Count(value, "\n") > 1 {
				expandYamlWildcard(value, wildcardLabels)
			} else {
				wildcardLabels[suffix] = value
			}
			continue
		}

		if suffix == "" || alias[0] == '#' {
			continue
		}

		if _, known := aliasSet[alias]; !known {
			aliasSet[alias] = len(aliasSet)
		}
	}

	if len(aliasSet) == 0 || len(wildcardLabels) == 0 {
		return
	}

	// Second pass: convert explicit labels to #N format
	for lbl, value := range labels {
		if !strings.HasPrefix(lbl, nsProxyDot) {
			continue
		}
		rest := lbl[len(nsProxyDot):]
		alias, suffix, ok := strings.Cut(rest, ".")
		if !ok || alias == "" || alias[0] == '#' {
			continue
		}

		idx := aliasSet[alias]

		delete(labels, lbl)
		if _, overridden := wildcardLabels[suffix]; !overridden {
			labels[refPrefix(idx)+suffix] = value
		}
	}

	// Expand wildcards for all aliases
	for suffix, value := range wildcardLabels {
		for _, idx := range aliasSet {
			labels[refPrefix(idx)+suffix] = value
		}
	}
}

// expandYamlWildcard parses a YAML document in value, flattens it to dot-notated keys and adds the
// results into dest map where each key is the flattened suffix and the value is the scalar string
// representation. The provided YAML is expected to be a mapping.
func expandYamlWildcard(value string, dest map[string]string) {
	// replace tab indentation with spaces to make YAML parser happy
	yamlStr := strings.ReplaceAll(value, "\t", "    ")

	raw := make(map[string]any)
	if err := yaml.Unmarshal([]byte(yamlStr), &raw); err != nil {
		// on parse error, ignore – treat as no-op
		return
	}

	flattenMap("", raw, dest)
}

// refPrefix returns the prefix for a reference to the Nth alias.
func refPrefix(n int) string {
	return nsProxyDot + "#" + strconv.Itoa(n+1) + "."
}

// flattenMap converts nested maps into a flat map with dot-delimited keys.
func flattenMap(prefix string, src map[string]any, dest map[string]string) {
	for k, v := range src {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch vv := v.(type) {
		case map[string]any:
			flattenMap(key, vv, dest)
		case map[any]any:
			flattenMapAny(key, vv, dest)
		case string:
			dest[key] = vv
		case int:
			dest[key] = strconv.Itoa(vv)
		case bool:
			dest[key] = strconv.FormatBool(vv)
		case float64:
			dest[key] = strconv.FormatFloat(vv, 'f', -1, 64)
		default:
			dest[key] = fmt.Sprint(v)
		}
	}
}

func flattenMapAny(prefix string, src map[any]any, dest map[string]string) {
	for k, v := range src {
		var key string
		switch kk := k.(type) {
		case string:
			key = kk
		default:
			key = fmt.Sprint(k)
		}
		if prefix != "" {
			key = prefix + "." + key
		}
		switch vv := v.(type) {
		case map[string]any:
			flattenMap(key, vv, dest)
		case map[any]any:
			flattenMapAny(key, vv, dest)
		case string:
			dest[key] = vv
		case int:
			dest[key] = strconv.Itoa(vv)
		case bool:
			dest[key] = strconv.FormatBool(vv)
		case float64:
			dest[key] = strconv.FormatFloat(vv, 'f', -1, 64)
		default:
			dest[key] = fmt.Sprint(v)
		}
	}
}

package docker

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

var ErrInvalidLabel = gperr.New("invalid label")

const nsProxyDot = NSProxy + "."

var refPrefixes = func() []string {
	prefixes := make([]string, 100)
	for i := range prefixes {
		prefixes[i] = nsProxyDot + "#" + strconv.Itoa(i+1) + "."
	}
	return prefixes
}()

func ParseLabels(labels map[string]string, aliases ...string) (types.LabelMap, gperr.Error) {
	nestedMap := make(types.LabelMap)
	errs := gperr.NewBuilder("labels error")

	ExpandWildcard(labels, aliases...)

	for lbl, value := range labels {
		parts := strutils.SplitRune(lbl, '.')
		if parts[0] != NSProxy {
			continue
		}
		if len(parts) == 1 {
			errs.Add(ErrInvalidLabel.Subject(lbl))
			continue
		}
		parts = parts[1:]
		currentMap := nestedMap

		for i, k := range parts {
			if i == len(parts)-1 {
				// Last element, set the value
				currentMap[k] = value
			} else {
				// If the key doesn't exist, create a new map
				if _, exists := currentMap[k]; !exists {
					currentMap[k] = make(types.LabelMap)
				}
				// Move deeper into the nested map
				m, ok := currentMap[k].(types.LabelMap)
				if !ok && currentMap[k] != "" {
					errs.Add(gperr.Errorf("expect mapping, got %T", currentMap[k]).Subject(lbl))
					continue
				} else if !ok {
					m = make(types.LabelMap)
					currentMap[k] = m
				}
				currentMap = m
			}
		}
	}

	return nestedMap, errs.Error()
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
		dotIdx := strings.IndexByte(rest, '.')
		var alias, suffix string
		if dotIdx == -1 {
			alias = rest
		} else {
			alias = rest[:dotIdx]
			suffix = rest[dotIdx+1:]
		}

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
		dotIdx := strings.IndexByte(rest, '.')
		if dotIdx == -1 {
			continue
		}
		alias := rest[:dotIdx]
		if alias[0] == '#' {
			continue
		}
		suffix := rest[dotIdx+1:]

		idx, known := aliasSet[alias]
		if !known {
			continue
		}

		delete(labels, lbl)
		if _, overridden := wildcardLabels[suffix]; !overridden {
			labels[refPrefixes[idx]+suffix] = value
		}
	}

	// Expand wildcards for all aliases
	for suffix, value := range wildcardLabels {
		for _, idx := range aliasSet {
			labels[refPrefixes[idx]+suffix] = value
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

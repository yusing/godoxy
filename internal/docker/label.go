package docker

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type LabelMap = map[string]any

var ErrInvalidLabel = gperr.New("invalid label")

func ParseLabels(labels map[string]string, aliases ...string) (LabelMap, gperr.Error) {
	nestedMap := make(LabelMap)
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
					currentMap[k] = make(LabelMap)
				}
				// Move deeper into the nested map
				m, ok := currentMap[k].(LabelMap)
				if !ok && currentMap[k] != "" {
					errs.Add(gperr.Errorf("expect mapping, got %T", currentMap[k]).Subject(lbl))
					continue
				} else if !ok {
					m = make(LabelMap)
					currentMap[k] = m
				}
				currentMap = m
			}
		}
	}

	return nestedMap, errs.Error()
}

func ExpandWildcard(labels map[string]string, aliases ...string) {
	// collect all explicit aliases first
	aliasSet := make(map[string]int, len(labels))
	// wildcardLabels holds mapping suffix -> value derived from wildcard label definitions
	wildcardLabels := make(map[string]string)

	for i, alias := range aliases {
		aliasSet[alias] = i
	}

	// iterate over a copy of the keys to safely mutate the map while ranging
	for lbl, value := range labels {
		parts := strings.SplitN(lbl, ".", 3)
		if len(parts) < 2 || parts[0] != NSProxy {
			continue
		}
		alias := parts[1]
		if alias == WildcardAlias { // "*"
			// remove wildcard label from original map – it should not remain afterwards
			delete(labels, lbl)

			// value looks like YAML (multiline)
			if strings.Count(value, "\n") > 1 {
				expandYamlWildcard(value, wildcardLabels)
				continue
			}

			// normal wildcard label with suffix – store directly
			wildcardLabels[parts[2]] = value
			continue
		}
		// explicit alias label – remember the alias
		if _, ok := aliasSet[alias]; !ok {
			aliasSet[alias] = len(aliasSet)
		}
	}

	if len(aliasSet) == 0 || len(wildcardLabels) == 0 {
		return // nothing to expand
	}

	// expand collected wildcard labels for every alias
	for suffix, v := range wildcardLabels {
		for alias, i := range aliasSet {
			// for FQDN aliases, use numeric index instead of the alias name
			if strings.Contains(alias, ".") {
				alias = fmt.Sprintf("#%d", i+1)
			}
			key := fmt.Sprintf("%s.%s.%s", NSProxy, alias, suffix)
			if suffix == "" { // this should not happen (root wildcard handled earlier) but keep safe
				key = fmt.Sprintf("%s.%s", NSProxy, alias)
			}
			labels[key] = v
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
			// convert to map[string]any by stringifying keys
			tmp := make(map[string]any, len(vv))
			for kk, vvv := range vv {
				tmp[fmt.Sprintf("%v", kk)] = vvv
			}
			flattenMap(key, tmp, dest)
		default:
			dest[key] = fmt.Sprint(v)
		}
	}
}

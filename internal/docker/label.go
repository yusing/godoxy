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
	strutils "github.com/yusing/goutils/strings"
)

var (
	ErrInvalidLabel          = errors.New("invalid label")
	ErrShortcutLabelConflict = errors.New("shortcut conflicts with full label")
)

const nsProxyDot = NSProxy + "."

type labelScalarShortcut struct {
	label string
	field string
}

var labelScalarShortcuts = []labelScalarShortcut{
	{"ports", "port"},
	{"protos", "scheme"},
	{"protocols", "scheme"},
	{"schemes", "scheme"},
	{"hosts", "host"},
	{"binds", "bind"},
	{"roots", "root"},
	{"spas", "spa"},
	{"indexes", "index"},
	{"no_tls_verifies", "no_tls_verify"},
	{"response_header_timeouts", "response_header_timeout"},
	{"max_conns_per_hosts", "max_conns_per_host"},
	{"disable_compressions", "disable_compression"},
	{"ssl_server_names", "ssl_server_name"},
	{"ssl_trusted_certificates", "ssl_trusted_certificate"},
	{"ssl_certificates", "ssl_certificate"},
	{"ssl_certificate_keys", "ssl_certificate_key"},
	{"agents", "agent"},
	{"inbound_mtls_profiles", "inbound_mtls_profile"},
	{"rule_files", "rule_file"},
	{"tls_terminations", "tls_termination"},
	{"proxy_protocol_headers", "relay_proxy_protocol_header"},
}

type UnexpectedTypeError struct {
	Expected string
	Actual   any
	// Message, if non-empty, is returned by Error() instead of the default "expect …, got …" form.
	Message string
}

func (e UnexpectedTypeError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("expect %s, got %T", e.Expected, e.Actual)
}

func ParseLabels(labels map[string]string, aliases ...string) (types.LabelMap, error) {
	nestedMap := make(types.LabelMap)
	errs := gperr.NewBuilder("labels error")

	errs.Add(ExpandScalarShortcuts(labels))
	ExpandWildcard(labels, aliases...)

	keys := slices.SortedFunc(maps.Keys(labels), compareLabelKeys)
	for _, lbl := range keys {
		if err := applyLabel(nestedMap, lbl, labels[lbl]); err != nil {
			errs.AddSubject(err, lbl)
		}
	}

	return nestedMap, errs.Error()
}

func ExpandScalarShortcuts(labels map[string]string) error {
	errs := gperr.NewBuilder("shortcut labels error")

	for _, shortcut := range labelScalarShortcuts {
		label := nsProxyDot + shortcut.label
		value, ok := labels[label]
		if !ok {
			continue
		}

		delete(labels, label)
		conflicts := shortcutLabelConflicts(labels, shortcut.field)
		if len(conflicts) != 0 {
			errs.AddSubjectf(
				ErrShortcutLabelConflict,
				"%s -> %s conflicts with %s",
				label,
				shortcut.field,
				strings.Join(conflicts, ", "),
			)
			continue
		}
		for i, item := range strutils.CommaSeperatedList(value) {
			labels[refPrefix(i)+shortcut.field] = item
		}
	}

	return errs.Error()
}

func shortcutLabelConflicts(labels map[string]string, field string) []string {
	conflicts := make([]string, 0)
	for label, value := range labels {
		_, suffix, ok := splitAliasLabel(label)
		if !ok {
			continue
		}
		if suffix == field || suffix == "" && labelObjectHasField(value, field) {
			conflicts = append(conflicts, label)
		}
	}

	slices.Sort(conflicts)
	return conflicts
}

func labelObjectHasField(value, field string) bool {
	flattened := make(map[string]string)
	expandYamlWildcard(value, flattened)
	_, ok := flattened[field]
	return ok
}

func applyLabel(dst types.LabelMap, lbl, value string) error {
	parts := strings.Split(lbl, ".")
	if parts[0] != NSProxy {
		return nil
	}
	if len(parts) == 1 {
		return ErrInvalidLabel
	}

	currentMap := dst
	for _, part := range parts[1 : len(parts)-1] {
		nextMap, err := descendLabelMap(currentMap, part)
		if err != nil {
			return err
		}
		currentMap = nextMap
	}

	return setLabelValue(currentMap, parts[len(parts)-1], value)
}

func descendLabelMap(currentMap types.LabelMap, key string) (types.LabelMap, error) {
	if next, ok := currentMap[key]; ok {
		switch typed := next.(type) {
		case types.LabelMap:
			return typed, nil
		case string:
			objectValue, isObject := parseLabelObject(typed)
			if !isObject {
				return nil, UnexpectedTypeError{Expected: "mapping", Actual: next}
			}
			currentMap[key] = objectValue
			return objectValue, nil
		default:
			return nil, UnexpectedTypeError{Expected: "mapping", Actual: next}
		}
	}

	nextMap := make(types.LabelMap)
	currentMap[key] = nextMap
	return nextMap, nil
}

func setLabelValue(currentMap types.LabelMap, key, value string) error {
	existing, ok := currentMap[key].(types.LabelMap)
	if !ok {
		currentMap[key] = value
		return nil
	}

	objectValue, isObject := parseLabelObject(value)
	if !isObject {
		return UnexpectedTypeError{Expected: "mapping", Actual: value}
	}
	return mergeLabelMaps(existing, objectValue)
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
			return UnexpectedTypeError{Expected: "mapping", Actual: srcValue}
		}
		if srcIsMap {
			return UnexpectedTypeError{
				Expected: "scalar",
				Actual:   srcValue,
				Message: fmt.Sprintf(
					"cannot merge mapping into existing scalar; merge source is %T",
					srcValue,
				),
			}
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
		alias, suffix, ok := splitAliasLabel(lbl)
		if !ok {
			continue
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
		alias, suffix, ok := splitAliasLabel(lbl)
		if !ok || suffix == "" || alias == "" || alias[0] == '#' {
			continue
		}
		idx, known := aliasSet[alias]
		if !known {
			continue
		}

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

func splitAliasLabel(lbl string) (alias, suffix string, ok bool) {
	rest, ok := strings.CutPrefix(lbl, nsProxyDot)
	if !ok {
		return "", "", false
	}
	alias, suffix, _ = strings.Cut(rest, ".")
	return alias, suffix, true
}

// expandYamlWildcard parses a YAML document in value, flattens it to dot-notated keys and adds the
// results into dest map where each key is the flattened suffix and the value is the scalar string
// representation. The provided YAML is expected to be a mapping.
func expandYamlWildcard(value string, dest map[string]string) {
	// replace tab indentation with spaces to make YAML parser happy
	yamlStr := strings.ReplaceAll(value, "\t", "  ")

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
		flattenValue(joinLabelKey(prefix, k), v, dest)
	}
}

func flattenMapAny(prefix string, src map[any]any, dest map[string]string) {
	for k, v := range src {
		flattenValue(joinLabelKey(prefix, stringifyLabelKey(k)), v, dest)
	}
}

func flattenValue(key string, value any, dest map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		flattenMap(key, typed, dest)
	case map[any]any:
		flattenMapAny(key, typed, dest)
	case string:
		dest[key] = typed
	case int:
		dest[key] = strconv.Itoa(typed)
	case bool:
		dest[key] = strconv.FormatBool(typed)
	case float64:
		dest[key] = strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		dest[key] = fmt.Sprint(value)
	}
}

func joinLabelKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func stringifyLabelKey(key any) string {
	if typed, ok := key.(string); ok {
		return typed
	}
	return fmt.Sprint(key)
}

package env

import (
	"os"
	"slices"
	"strings"
)

// Setting is a safe diagnostic record. Sensitive values are never retained.
type Setting struct {
	Name   string
	Source string
	Value  any
}

// Diagnostics contains only safe values and variable names, never secret values.
type Diagnostics struct {
	Settings   []Setting
	Unknown    []string
	Deprecated []string
}

// Inspect reports the effective server environment (not per-route config
// overrides). Call once at startup, after initializing the application's logger.
func Inspect() Diagnostics {
	var result Diagnostics
	known := make(map[string]bool, len(definitions))
	for _, d := range definitions {
		if d.Compose {
			known[d.Name] = true
			continue
		}
		name := "GODOXY_" + d.Name
		known[name] = true
		_, source, _ := LookupEnvSource(d.Name)
		if source == d.Name {
			result.Deprecated = append(result.Deprecated, d.Name)
		}
		if source == "" {
			source = "default"
		}
		setting := Setting{Name: name, Source: source}
		if d.Sensitive {
			setting.Value = "[redacted]"
		} else {
			switch d.Type {
			case "bool":
				setting.Value = Bool(d.Name)
			case "int":
				setting.Value = Int(d.Name)
			case "duration":
				setting.Value = Duration(d.Name).String()
			case "list":
				setting.Value = CommaSep(d.Name)
			default:
				setting.Value = String(d.Name)
			}
		}
		result.Settings = append(result.Settings, setting)
	}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GODOXY_") && !known[name] {
			result.Unknown = append(result.Unknown, name)
		}
	}
	slices.Sort(result.Unknown)
	slices.Sort(result.Deprecated)
	return result
}

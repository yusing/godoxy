package main

import (
	"encoding/json/v2"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/yusing/godoxy/internal/autocert"
)

func resolveSchema(s schema, defs map[string]schema) schema {
	if name, ok := strings.CutPrefix(s.Ref, "#/definitions/"); ok {
		return defs[name]
	}
	return s
}

func optionSchemaTypes(s schema, defs map[string]schema) []string {
	s = resolveSchema(s, defs)
	var types []string
	if len(s.Type) > 0 {
		if s.Type.Kind() == '"' {
			var name string
			if json.Unmarshal(s.Type, &name) == nil {
				types = append(types, name)
			}
		} else {
			_ = json.Unmarshal(s.Type, &types)
		}
	}
	for _, branch := range s.AnyOf {
		types = append(types, optionSchemaTypes(branch, defs)...)
	}
	slices.Sort(types)
	return slices.Compact(types)
}

func checkOptionSchema(t reflect.Type, s schema, defs map[string]schema) error {
	if t.Kind() == reflect.Pointer {
		return checkOptionSchema(t.Elem(), s, defs)
	}
	s = resolveSchema(s, defs)
	want := []string{"string"}
	if t != reflect.TypeFor[time.Duration]() {
		switch t.Kind() {
		case reflect.Bool:
			want = []string{"boolean", "string"}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			want = []string{"number", "string"}
		case reflect.Slice, reflect.Array:
			want = []string{"array"}
			if s.Items == nil {
				return fmt.Errorf("array has no concrete item schema")
			}
			if err := checkOptionSchema(t.Elem(), *s.Items, defs); err != nil {
				return err
			}
		case reflect.Map:
			want = []string{"object"}
			var value schema
			if err := json.Unmarshal(s.Additional, &value); err != nil {
				return fmt.Errorf("map has no concrete value schema")
			}
			if err := checkOptionSchema(t.Elem(), value, defs); err != nil {
				return err
			}
		case reflect.Struct:
			want = []string{"object"}
			fields := optionFields(t)
			if len(s.Properties) != len(fields) || string(s.Additional) != "false" {
				return fmt.Errorf("%s must have exactly its concrete fields", t)
			}
			for _, field := range fields {
				name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
				if name == "" {
					name = optionKey(field.Name)
				}
				property, ok := s.Properties[name]
				if !ok {
					return fmt.Errorf("missing field %s", name)
				}
				if err := checkOptionSchema(field.Type, property, defs); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
			}
		}
	}
	if got := optionSchemaTypes(s, defs); !slices.Equal(got, want) {
		return fmt.Errorf("%s requires %v, got %v", t, want, got)
	}
	return nil
}

func checkProviderOptionSchemas(s schema, defs map[string]schema) error {
	s = resolveSchema(s, defs)
	if len(s.AnyOf) > 0 {
		for _, branch := range s.AnyOf {
			if err := checkProviderOptionSchemas(branch, defs); err != nil {
				return err
			}
		}
		return nil
	}
	provider := s.Properties["provider"].Const
	if provider == autocert.ProviderLocal || provider == autocert.ProviderCustom {
		return nil
	}
	factory, ok := autocert.Providers[provider]
	if !ok || factory.ConfigType == nil {
		return fmt.Errorf("unknown concrete provider %q", provider)
	}
	if err := checkOptionSchema(factory.ConfigType, s.Properties["options"], defs); err != nil {
		return fmt.Errorf("%s: %w", provider, err)
	}
	return nil
}

package main

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/yusing/godoxy/internal/autocert"
)

func optionKey(name string) string {
	name = strings.ReplaceAll(name, "OAuth", "Oauth")
	runes := []rune(name)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) &&
			(unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) ||
				(i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func optionFields(t reflect.Type) []reflect.StructField {
	var fields []reflect.StructField
	for i := range t.NumField() {
		field := t.Field(i)
		jsonName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if !field.IsExported() || jsonName == "-" || field.Tag.Get("deserialize") == "-" ||
			field.Type == reflect.TypeFor[*http.Client]() || field.Type.Kind() == reflect.Interface {
			continue // Transport and SDK credential objects are not configuration values.
		}
		fieldType := field.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if field.Anonymous && fieldType.Kind() == reflect.Struct {
			fields = append(fields, optionFields(fieldType)...)
			continue
		}
		if fieldType.Kind() == reflect.Struct && len(optionFields(fieldType)) == 0 {
			continue
		}
		fields = append(fields, field)
	}
	return fields
}

func optionType(t reflect.Type, indent string) (string, error) {
	if t.Kind() == reflect.Pointer {
		return optionType(t.Elem(), indent)
	}
	if t == reflect.TypeFor[time.Duration]() {
		return "ProviderDuration", nil
	}
	switch t.Kind() {
	case reflect.String:
		return "string", nil
	case reflect.Bool:
		return "boolean | 'true' | 'false'", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number | `${number}`", nil
	case reflect.Slice, reflect.Array:
		elem, err := optionType(t.Elem(), indent)
		return "Array<" + elem + ">", err
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return "", fmt.Errorf("unsupported option map key %s", t.Key())
		}
		elem, err := optionType(t.Elem(), indent+"  ")
		return "{\n" + indent + "  [key: string]: " + elem + "\n" + indent + "}", err
	case reflect.Struct:
		var b strings.Builder
		b.WriteString("{\n")
		for _, field := range optionFields(t) {
			fieldType, err := optionType(field.Type, indent+"  ")
			if err != nil {
				return "", fmt.Errorf("%s.%s: %w", t, field.Name, err)
			}
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" {
				name = optionKey(field.Name)
			}
			fmt.Fprintf(&b, "%s  %s?: %s\n", indent, name, fieldType)
		}
		fmt.Fprintf(&b, "%s}", indent)
		return b.String(), nil
	default:
		return "", fmt.Errorf("unsupported configurable option type %s", t)
	}
}

func renderTypes(names []string) ([]byte, error) {
	var b strings.Builder
	b.Write(renderType(names))
	fmt.Fprintln(&b, "import type { AutocertConfigBase, LocalOptions, CustomOptions } from './autocert'")
	fmt.Fprintln(&b, "\n// Go duration, for example 30s or 1m30s.\nexport type ProviderDuration = string")
	var variants []string
	for _, name := range names {
		if name == autocert.ProviderLocal || name == autocert.ProviderCustom {
			continue
		}
		factory := autocert.Providers[name]
		if factory.ConfigType == nil || factory.ConfigType.Kind() != reflect.Struct {
			return nil, fmt.Errorf("%s has no concrete provider configuration type", name)
		}
		fields, err := optionType(factory.ConfigType, "  ")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		variant := strings.ToUpper(name[:1]) + name[1:] + "Options"
		variants = append(variants, variant)
		fmt.Fprintf(&b, "\nexport interface %s extends AutocertConfigBase {\n  provider: '%s'\n  options?: %s\n}\n", variant, name, fields)
	}
	fmt.Fprintln(&b, "\nexport type AutocertConfigWithoutExtra =\n  | LocalOptions\n  | CustomOptions")
	for _, variant := range variants {
		fmt.Fprintf(&b, "  | %s\n", variant)
	}
	fmt.Fprintln(&b, "\nexport type AutocertExtra =\n  | Partial<LocalOptions>\n  | Partial<CustomOptions>")
	for _, variant := range variants {
		fmt.Fprintf(&b, "  | Partial<%s>\n", variant)
	}
	return []byte(b.String()), nil
}

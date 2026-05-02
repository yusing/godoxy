package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type swaggerSpec struct {
	BasePath    string                          `json:"basePath"`
	Paths       map[string]map[string]operation `json:"paths"`
	Definitions map[string]definition           `json:"definitions"`
}

type operation struct {
	OperationID string      `json:"operationId"`
	Summary     string      `json:"summary"`
	Tags        []string    `json:"tags"`
	Parameters  []parameter `json:"parameters"`
}

type parameter struct {
	Name        string     `json:"name"`
	In          string     `json:"in"`
	Required    bool       `json:"required"`
	Type        string     `json:"type"`
	Description string     `json:"description"`
	Schema      *schemaRef `json:"schema"`
}

type schemaRef struct {
	Ref string `json:"$ref"`
}

type definition struct {
	Type       string                `json:"type"`
	Required   []string              `json:"required"`
	Properties map[string]definition `json:"properties"`
	Items      *definition           `json:"items"`
}

type endpoint struct {
	CommandPath []string
	Method      string
	Path        string
	Summary     string
	IsWebSocket bool
	Params      []param
}

type param struct {
	FlagName    string
	Name        string
	In          string
	Type        string
	Required    bool
	Description string
}

func main() {
	root := filepath.Join("..", "..")
	inPath := filepath.Join(root, "internal", "api", "v1", "docs", "swagger.json")
	outPath := "generated_commands.go"

	raw, err := os.ReadFile(inPath)
	must(err)

	var spec swaggerSpec
	must(json.Unmarshal(raw, &spec))

	eps := buildEndpoints(spec)
	must(writeGenerated(outPath, eps))
}

func buildEndpoints(spec swaggerSpec) []endpoint {
	byCommand := map[string]endpoint{}

	pathKeys := make([]string, 0, len(spec.Paths))
	for p := range spec.Paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)

	for _, p := range pathKeys {
		methodMap := spec.Paths[p]
		methods := make([]string, 0, len(methodMap))
		for m := range methodMap {
			methods = append(methods, strings.ToUpper(m))
		}
		sort.Strings(methods)

		for _, method := range methods {
			op := methodMap[strings.ToLower(method)]
			if op.OperationID == "" {
				continue
			}
			ep := endpoint{
				CommandPath: commandPathFromOp(p, op.OperationID),
				Method:      method,
				Path:        ensureSlash(spec.BasePath) + normalizePath(p),
				Summary:     op.Summary,
				IsWebSocket: hasTag(op.Tags, "websocket"),
				Params:      collectParams(spec, op),
			}
			key := strings.Join(ep.CommandPath, " ")
			if existing, ok := byCommand[key]; ok {
				if betterEndpoint(ep, existing) {
					byCommand[key] = ep
				}
				continue
			}
			byCommand[key] = ep
		}
	}

	out := make([]endpoint, 0, len(byCommand))
	for _, ep := range byCommand {
		out = append(out, ep)
	}
	sort.Slice(out, func(i, j int) bool {
		ai := strings.Join(out[i].CommandPath, " ")
		aj := strings.Join(out[j].CommandPath, " ")
		return ai < aj
	})
	return out
}

func commandPathFromOp(path, opID string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return []string{toKebab(opID)}
	}
	if len(parts) == 1 {
		return []string{toKebab(parts[0])}
	}
	group := toKebab(parts[0])
	name := toKebab(opID)
	if name == group {
		name = "get"
	}
	if group == "v1" {
		return []string{name}
	}
	return []string{group, name}
}

func collectParams(spec swaggerSpec, op operation) []param {
	params := make([]param, 0)
	for _, p := range op.Parameters {
		switch p.In {
		case "body":
			if p.Schema != nil && p.Schema.Ref != "" {
				defName := strings.TrimPrefix(p.Schema.Ref, "#/definitions/")
				params = append(params, bodyParamsFromDef(spec.Definitions[defName])...)
				continue
			}
			params = append(params, param{
				FlagName:    toKebab(p.Name),
				Name:        p.Name,
				In:          "body",
				Type:        defaultType(p.Type),
				Required:    p.Required,
				Description: p.Description,
			})
		default:
			params = append(params, param{
				FlagName:    toKebab(p.Name),
				Name:        p.Name,
				In:          p.In,
				Type:        defaultType(p.Type),
				Required:    p.Required,
				Description: p.Description,
			})
		}
	}

	// Deduplicate by flag name, prefer required entries.
	byFlag := map[string]param{}
	for _, p := range params {
		if cur, ok := byFlag[p.FlagName]; ok {
			if !cur.Required && p.Required {
				byFlag[p.FlagName] = p
			}
			continue
		}
		byFlag[p.FlagName] = p
	}
	out := make([]param, 0, len(byFlag))
	for _, p := range byFlag {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].In != out[j].In {
			return out[i].In < out[j].In
		}
		return out[i].FlagName < out[j].FlagName
	})
	return out
}

func bodyParamsFromDef(def definition) []param {
	if def.Type != "object" {
		return nil
	}
	requiredSet := map[string]struct{}{}
	for _, name := range def.Required {
		requiredSet[name] = struct{}{}
	}
	keys := make([]string, 0, len(def.Properties))
	for k := range def.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]param, 0, len(keys))
	for _, k := range keys {
		prop := def.Properties[k]
		_, required := requiredSet[k]
		t := defaultType(prop.Type)
		if prop.Type == "array" {
			t = "array"
		}
		if prop.Type == "object" {
			t = "object"
		}
		out = append(out, param{
			FlagName: toKebab(k),
			Name:     k,
			In:       "body",
			Type:     t,
			Required: required,
		})
	}
	return out
}

func betterEndpoint(a, b endpoint) bool {
	// Prefer GET, then fewer path params, then shorter path.
	if a.Method == "GET" && b.Method != "GET" {
		return true
	}
	if a.Method != "GET" && b.Method == "GET" {
		return false
	}
	ac := countPathParams(a.Path)
	bc := countPathParams(b.Path)
	if ac != bc {
		return ac < bc
	}
	return len(a.Path) < len(b.Path)
}

func countPathParams(path string) int {
	count := 0
	for seg := range strings.SplitSeq(path, "/") {
		if strings.HasPrefix(seg, "{") || strings.HasPrefix(seg, ":") {
			count++
		}
	}
	return count
}

func normalizePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			parts[i] = "{" + name + "}"
		}
	}
	return strings.Join(parts, "/")
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

func writeGenerated(outPath string, eps []endpoint) error {
	var b bytes.Buffer
	b.WriteString("// Code generated by cmd/cli/gen. DO NOT EDIT.\n")
	b.WriteString("package main\n\n")
	b.WriteString("var generatedEndpoints = []Endpoint{\n")
	for _, ep := range eps {
		b.WriteString("\t{\n")
		fmt.Fprintf(&b, "\t\tCommandPath: %#v,\n", ep.CommandPath)
		fmt.Fprintf(&b, "\t\tMethod: %q,\n", ep.Method)
		fmt.Fprintf(&b, "\t\tPath: %q,\n", ep.Path)
		fmt.Fprintf(&b, "\t\tSummary: %q,\n", ep.Summary)
		fmt.Fprintf(&b, "\t\tIsWebSocket: %t,\n", ep.IsWebSocket)
		b.WriteString("\t\tParams: []Param{\n")
		for _, p := range ep.Params {
			b.WriteString("\t\t\t{\n")
			fmt.Fprintf(&b, "\t\t\t\tFlagName: %q,\n", p.FlagName)
			fmt.Fprintf(&b, "\t\t\t\tName: %q,\n", p.Name)
			fmt.Fprintf(&b, "\t\t\t\tIn: %q,\n", p.In)
			fmt.Fprintf(&b, "\t\t\t\tType: %q,\n", p.Type)
			fmt.Fprintf(&b, "\t\t\t\tRequired: %t,\n", p.Required)
			fmt.Fprintf(&b, "\t\t\t\tDescription: %q,\n", p.Description)
			b.WriteString("\t\t\t},\n")
		}
		b.WriteString("\t\t},\n")
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")
	formatted, err := format.Source(b.Bytes())
	if err != nil {
		return fmt.Errorf("format generated source: %w", err)
	}
	return os.WriteFile(outPath, formatted, 0o644)
}

func ensureSlash(s string) string {
	if strings.HasPrefix(s, "/") {
		return s
	}
	return "/" + s
}

func defaultType(t string) string {
	switch t {
	case "integer", "number", "boolean", "array", "object", "string":
		return t
	default:
		return "string"
	}
}

func toKebab(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, ".", "-")
	var out []rune
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 && out[len(out)-1] != '-' {
				out = append(out, '-')
			}
			out = append(out, unicode.ToLower(r))
			continue
		}
		out = append(out, unicode.ToLower(r))
	}
	res := strings.Trim(string(out), "-")
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	return res
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

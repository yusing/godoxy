package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var trailingCommaEgPhraseRE = regexp.MustCompile(`(?i),\s*e\.g\.:?\s*$`)

// Suffixes ending in a period where the final dot is part of an abbreviation, not a sentence end.
var cheatsheetPreserveFinalPeriodSuffixes = []string{
	"approx.", "e.g.", "etc.", "fig.", "i.e.", "viz.", "vs.",
}

type (
	cheatsheet struct {
		Sections []section `json:"sections"`
	}

	section struct {
		ID          string  `json:"id"`
		Title       string  `json:"title"`
		Description string  `json:"description,omitempty"`
		Entries     []entry `json:"entries"`
	}

	entry struct {
		ID               string   `json:"id"`
		Name             string   `json:"name"`
		Kind             string   `json:"kind"`
		Summary          string   `json:"summary,omitempty"`
		Description      []string `json:"description,omitempty"`
		Syntax           string   `json:"syntax,omitempty"`
		Examples         []string `json:"examples,omitempty"`
		Args             []arg    `json:"args,omitempty"`
		Aliases          []string `json:"aliases,omitempty"`
		Phase            string   `json:"phase,omitempty"`
		Terminates       bool     `json:"terminates,omitempty"`
		SupportedActions []string `json:"supportedActions,omitempty"`
		Tags             []string `json:"tags,omitempty"`
	}

	arg struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	helpData struct {
		Command     string
		Description []string
		Args        []arg
	}

	commandData struct {
		Key       string
		Help      helpData
		Validate  ast.Expr
		Build     *ast.FuncLit
		Terminate bool
	}

	checkerData struct {
		Key      string
		Help     helpData
		Validate ast.Expr
	}

	fieldData struct {
		Key      string
		Help     helpData
		Validate ast.Expr
		Build    *ast.FuncLit
	}

	dynamicVarData struct {
		Key   string
		Help  helpData
		Phase string
	}

	staticVarData struct {
		Key  string
		Help helpData
	}
)

type extractor struct {
	fset           *token.FileSet
	files          map[string]*ast.File
	funcs          map[string]*ast.FuncDecl
	consts         map[string]string
	commands       map[string]commandData
	checkers       map[string]checkerData
	fields         map[string]fieldData
	staticReqVars  map[string]staticVarData
	staticRespVars map[string]staticVarData
	dynamicVars    map[string]dynamicVarData
	allFields      []string
	matcherTypes   []string
	controlDocs    map[string][]string
}

func main() {
	outPath := flag.String("out", "", "output file")
	rulesDir := flag.String("rules-dir", "internal/route/rules", "rules source directory")
	flag.Parse()

	if outPath == nil || *outPath == "" {
		log.Fatal("-out required")
	}

	ex, err := parseRulesDir(*rulesDir)
	if err != nil {
		log.Fatal(err)
	}

	doc := ex.buildCheatsheet()

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		log.Fatal(err)
	}

	outFile, err := os.Create(*outPath)
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()

	enc := json.NewEncoder(outFile)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		log.Fatal(err)
	}
}

func parseRulesDir(dir string) (*extractor, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		name := info.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	pkg, ok := pkgs["rules"]
	if !ok {
		return nil, fmt.Errorf("package rules not found in %s", dir)
	}

	ex := &extractor{
		fset:           fset,
		files:          pkg.Files,
		funcs:          map[string]*ast.FuncDecl{},
		consts:         map[string]string{},
		commands:       map[string]commandData{},
		checkers:       map[string]checkerData{},
		fields:         map[string]fieldData{},
		staticReqVars:  map[string]staticVarData{},
		staticRespVars: map[string]staticVarData{},
		dynamicVars:    map[string]dynamicVarData{},
		matcherTypes:   []string{},
		controlDocs:    map[string][]string{},
	}

	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			switch n := decl.(type) {
			case *ast.GenDecl:
				ex.collectConsts(n)
				ex.collectVars(n)
				ex.collectControlDocs(n)
			case *ast.FuncDecl:
				ex.funcs[n.Name.Name] = n
			}
		}
	}

	ex.applyCommandAliases()
	return ex, nil
}

func (ex *extractor) collectConsts(decl *ast.GenDecl) {
	if decl.Tok != token.CONST {
		return
	}
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for i, name := range valueSpec.Names {
			if i >= len(valueSpec.Values) {
				continue
			}
			value, ok := ex.stringValue(valueSpec.Values[i])
			if !ok {
				continue
			}
			ex.consts[name.Name] = value
			if strings.HasPrefix(name.Name, "MatcherType") {
				ex.matcherTypes = append(ex.matcherTypes, value)
			}
		}
	}
}

func (ex *extractor) collectVars(decl *ast.GenDecl) {
	if decl.Tok != token.VAR {
		return
	}
	for _, spec := range decl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok || len(valueSpec.Names) != 1 || len(valueSpec.Values) != 1 {
			continue
		}
		name := valueSpec.Names[0].Name
		switch name {
		case "commands":
			ex.commands = ex.parseCommands(valueSpec.Values[0])
		case "checkers":
			ex.checkers = ex.parseCheckers(valueSpec.Values[0])
		case "modFields":
			ex.fields = ex.parseFields(valueSpec.Values[0])
		case "staticReqVarSubsMap":
			ex.staticReqVars = ex.parseStaticVars(valueSpec.Values[0])
		case "staticRespVarSubsMap":
			ex.staticRespVars = ex.parseStaticVars(valueSpec.Values[0])
		case "dynamicVarSubsMap":
			ex.dynamicVars = ex.parseDynamicVars(valueSpec.Values[0])
		case "AllFields":
			ex.allFields = ex.parseStringSlice(valueSpec.Values[0])
		}
	}
}

func (ex *extractor) collectControlDocs(decl *ast.GenDecl) {
	if decl.Tok != token.TYPE {
		return
	}
	for _, spec := range decl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		if typeSpec.Name.Name != "IfBlockCommand" && typeSpec.Name.Name != "IfElseBlockCommand" {
			continue
		}
		doc := decl.Doc
		if typeSpec.Doc != nil {
			doc = typeSpec.Doc
		}
		if doc == nil {
			continue
		}
		ex.controlDocs[typeSpec.Name.Name] = extractCommentLines(doc)
	}
}

func (ex *extractor) parseCommands(expr ast.Expr) map[string]commandData {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	result := map[string]commandData{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ex.stringValue(kv.Key)
		if !ok {
			continue
		}
		itemLit, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		item := commandData{Key: key}
		for _, fieldExpr := range itemLit.Elts {
			fieldKV, ok := fieldExpr.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			fieldName := identName(fieldKV.Key)
			switch fieldName {
			case "help":
				item.Help = ex.parseHelp(fieldKV.Value)
			case "validate":
				item.Validate = fieldKV.Value
			case "build":
				item.Build = ex.funcLitFromExpr(fieldKV.Value)
			case "terminate":
				item.Terminate = ex.boolValue(fieldKV.Value)
			}
		}
		if item.Help.Command == "" {
			item.Help.Command = key
		}
		result[key] = item
	}
	return result
}

func (ex *extractor) parseCheckers(expr ast.Expr) map[string]checkerData {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	result := map[string]checkerData{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ex.stringValue(kv.Key)
		if !ok {
			continue
		}
		itemLit, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		item := checkerData{Key: key}
		for _, fieldExpr := range itemLit.Elts {
			fieldKV, ok := fieldExpr.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			switch identName(fieldKV.Key) {
			case "help":
				item.Help = ex.parseHelp(fieldKV.Value)
			case "validate":
				item.Validate = fieldKV.Value
			}
		}
		if item.Help.Command == "" {
			item.Help.Command = key
		}
		result[key] = item
	}
	return result
}

func (ex *extractor) parseFields(expr ast.Expr) map[string]fieldData {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	result := map[string]fieldData{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ex.stringValue(kv.Key)
		if !ok {
			continue
		}
		itemLit, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		item := fieldData{Key: key}
		for _, fieldExpr := range itemLit.Elts {
			fieldKV, ok := fieldExpr.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			switch identName(fieldKV.Key) {
			case "help":
				item.Help = ex.parseHelp(fieldKV.Value)
			case "validate":
				item.Validate = fieldKV.Value
			case "builder":
				item.Build = ex.funcLitFromExpr(fieldKV.Value)
			}
		}
		if item.Help.Command == "" {
			item.Help.Command = key
		}
		result[key] = item
	}
	return result
}

func (ex *extractor) parseDynamicVars(expr ast.Expr) map[string]dynamicVarData {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	result := map[string]dynamicVarData{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ex.stringValue(kv.Key)
		if !ok {
			continue
		}
		itemLit, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		item := dynamicVarData{Key: key, Phase: "pre"}
		for _, fieldExpr := range itemLit.Elts {
			fieldKV, ok := fieldExpr.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			switch identName(fieldKV.Key) {
			case "help":
				item.Help = ex.parseHelp(fieldKV.Value)
			case "phase":
				item.Phase = ex.phaseFromExpr(fieldKV.Value)
			}
		}
		if item.Help.Command == "" {
			item.Help.Command = "$" + key
		}
		result[key] = item
	}
	return result
}

func (ex *extractor) parseStaticVars(expr ast.Expr) map[string]staticVarData {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	result := map[string]staticVarData{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ex.stringValue(kv.Key)
		if !ok {
			continue
		}
		itemLit, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		item := staticVarData{Key: key}
		for _, fieldExpr := range itemLit.Elts {
			fieldKV, ok := fieldExpr.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if identName(fieldKV.Key) == "help" {
				item.Help = ex.parseHelp(fieldKV.Value)
			}
		}
		if item.Help.Command == "" {
			item.Help.Command = "$" + key
		}
		result[key] = item
	}
	return result
}

func (ex *extractor) parseMapKeys(expr ast.Expr) []string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var keys []string
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ex.stringValue(kv.Key)
		if !ok {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func (ex *extractor) parseStringSlice(expr ast.Expr) []string {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	items := make([]string, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		if value, ok := ex.stringValue(elt); ok {
			items = append(items, value)
		}
	}
	return items
}

func (ex *extractor) parseHelp(expr ast.Expr) helpData {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return helpData{}
	}
	help := helpData{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		switch identName(kv.Key) {
		case "command":
			help.Command, _ = ex.stringValue(kv.Value)
		case "description":
			help.Description = ex.parseDescription(kv.Value)
		case "args":
			help.Args = ex.parseArgs(kv.Value)
		}
	}
	return help
}

func (ex *extractor) parseDescription(expr ast.Expr) []string {
	return ex.parseLines(expr)
}

func (ex *extractor) parseLines(expr ast.Expr) []string {
	switch n := expr.(type) {
	case *ast.CallExpr:
		if identName(n.Fun) != "makeLines" {
			return nil
		}
		lines := make([]string, 0, len(n.Args))
		for _, argExpr := range n.Args {
			if line, ok := ex.renderStringExpr(argExpr); ok {
				line = strings.TrimSpace(line)
				if line != "" {
					lines = append(lines, line)
				}
			}
		}
		return lines
	case *ast.CompositeLit:
		lines := make([]string, 0, len(n.Elts))
		for _, elt := range n.Elts {
			if line, ok := ex.renderStringExpr(elt); ok {
				line = strings.TrimSpace(line)
				if line != "" {
					lines = append(lines, line)
				}
			}
		}
		return lines
	default:
		line, ok := ex.renderStringExpr(expr)
		if !ok {
			return nil
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return nil
		}
		return []string{line}
	}
}

func (ex *extractor) parseArgs(expr ast.Expr) []arg {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	args := make([]arg, 0, len(lit.Elts))
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := ex.stringValue(kv.Key)
		if !ok {
			continue
		}
		desc, ok := ex.renderStringExpr(kv.Value)
		if !ok {
			continue
		}
		args = append(args, arg{Name: name, Description: desc})
	}
	return args
}

func (ex *extractor) renderStringExpr(expr ast.Expr) (string, bool) {
	switch n := expr.(type) {
	case *ast.BasicLit:
		value, err := strconv.Unquote(n.Value)
		if err != nil {
			return "", false
		}
		return value, true
	case *ast.Ident:
		value, ok := ex.consts[n.Name]
		return value, ok
	case *ast.CallExpr:
		switch identName(n.Fun) {
		case "helpExample":
			return ex.renderHelpExample(n)
		case "helpListItem":
			return ex.renderHelpListItem(n)
		case "helpFuncCall":
			return ex.renderHelpFuncCall(n)
		case "helpVar":
			if len(n.Args) != 1 {
				return "", false
			}
			return ex.renderStringExpr(n.Args[0])
		}
		if selector, ok := n.Fun.(*ast.SelectorExpr); ok {
			if identName(selector.X) == "strings" && selector.Sel.Name == "Join" && len(n.Args) == 2 {
				if identName(n.Args[0]) == "AllFields" {
					delim, ok := ex.renderStringExpr(n.Args[1])
					if !ok {
						return "", false
					}
					return strings.Join(ex.allFields, delim), true
				}
			}
		}
	case *ast.BinaryExpr:
		if n.Op != token.ADD {
			return "", false
		}
		left, ok := ex.renderStringExpr(n.X)
		if !ok {
			return "", false
		}
		right, ok := ex.renderStringExpr(n.Y)
		if !ok {
			return "", false
		}
		return left + right, true
	}
	return "", false
}

func (ex *extractor) renderHelpExample(call *ast.CallExpr) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	cmd, ok := ex.renderStringExpr(call.Args[0])
	if !ok {
		return "", false
	}
	parts := []string{cmd}
	for _, argExpr := range call.Args[1:] {
		if nestedCall, ok := argExpr.(*ast.CallExpr); ok && identName(nestedCall.Fun) == "helpFuncCall" {
			part, ok := ex.renderHelpFuncCall(nestedCall)
			if !ok {
				return "", false
			}
			parts = append(parts, part)
			continue
		}
		part, ok := ex.renderStringExpr(argExpr)
		if !ok {
			return "", false
		}
		parts = append(parts, fmt.Sprintf("%q", part))
	}
	return strings.Join(parts, " "), true
}

func (ex *extractor) renderHelpListItem(call *ast.CallExpr) (string, bool) {
	if len(call.Args) != 2 {
		return "", false
	}
	key, ok := ex.renderStringExpr(call.Args[0])
	if !ok {
		return "", false
	}
	value, ok := ex.renderStringExpr(call.Args[1])
	if !ok {
		return "", false
	}
	return key + ": " + value, true
}

func (ex *extractor) renderHelpFuncCall(call *ast.CallExpr) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	fn, ok := ex.renderStringExpr(call.Args[0])
	if !ok {
		return "", false
	}
	args := make([]string, 0, len(call.Args)-1)
	for _, argExpr := range call.Args[1:] {
		value, ok := ex.renderStringExpr(argExpr)
		if !ok {
			return "", false
		}
		args = append(args, fmt.Sprintf("%q", value))
	}
	return fn + "(" + strings.Join(args, ", ") + ")", true
}

func (ex *extractor) buildCheatsheet() cheatsheet {
	return cheatsheet{
		Sections: []section{
			ex.buildMatcherSection(),
			ex.buildCommandSection(),
			ex.buildFieldSection(),
			ex.buildVariableSection(),
			ex.buildPatternSection(),
			ex.buildControlSection(),
		},
	}
}

func (ex *extractor) buildCommandSection() section {
	entriesByName := map[string]*entry{}
	for key, item := range ex.commands {
		name := item.Help.Command
		if name == "" {
			name = key
		}
		if existing, ok := entriesByName[name]; ok {
			if key != name {
				existing.Aliases = append(existing.Aliases, key)
			}
			continue
		}

		e := ex.entryFromHelp("command", name, item.Help)
		e.Phase = ex.inferCommandPhase(item.Validate)
		e.Terminates = item.Terminate
		e.Tags = compact([]string{"command", e.Phase})
		entriesByName[name] = &e
		if key != name {
			entriesByName[name].Aliases = append(entriesByName[name].Aliases, key)
		}
	}

	entries := mapsToSortedEntries(entriesByName)
	return section{
		ID:      "commands",
		Title:   "Actions",
		Entries: entries,
	}
}

func (ex *extractor) buildMatcherSection() section {
	entries := make([]entry, 0, len(ex.checkers))
	for _, name := range sortedKeys(ex.checkers) {
		item := ex.checkers[name]
		e := ex.entryFromHelp("matcher", name, item.Help)
		e.Phase = ex.inferMatcherPhase(item.Validate)
		e.Tags = compact([]string{"matcher", e.Phase})
		entries = append(entries, e)
	}
	return section{
		ID:      "matchers",
		Title:   "Conditions",
		Entries: entries,
	}
}

func (ex *extractor) buildFieldSection() section {
	entries := make([]entry, 0, len(ex.fields))
	for _, name := range sortedKeys(ex.fields) {
		item := ex.fields[name]
		e := ex.entryFromHelp("field", name, item.Help)
		e.Phase = ex.inferFieldPhase(item.Validate)
		e.SupportedActions = ex.inferSupportedActions(item.Build)
		e.Tags = append([]string{"mutation-field", e.Phase}, e.SupportedActions...)
		entries = append(entries, e)
	}
	return section{
		ID:      "mutation-fields",
		Title:   "Mutation Targets",
		Entries: entries,
	}
}

func (ex *extractor) buildVariableSection() section {
	entries := make([]entry, 0, len(ex.staticReqVars)+len(ex.staticRespVars)+len(ex.dynamicVars))

	for _, name := range sortedKeys(ex.staticReqVars) {
		item := ex.staticReqVars[name]
		e := ex.entryFromHelp("variable", item.Help.Command, item.Help)
		e.ID = "var-" + name
		e.Phase = "pre"
		e.Tags = []string{"variable", "request"}
		entries = append(entries, e)
	}
	for _, name := range sortedKeys(ex.staticRespVars) {
		item := ex.staticRespVars[name]
		e := ex.entryFromHelp("variable", item.Help.Command, item.Help)
		e.ID = "var-" + name
		e.Phase = "post"
		e.Tags = []string{"variable", "response"}
		entries = append(entries, e)
	}
	for _, name := range sortedKeys(ex.dynamicVars) {
		item := ex.dynamicVars[name]
		e := ex.entryFromHelp("variable", item.Help.Command, item.Help)
		e.ID = "var-" + name
		e.Phase = item.Phase
		e.Tags = []string{"variable", "dynamic"}
		entries = append(entries, e)
	}

	slices.SortFunc(entries, func(a, b entry) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})

	return section{
		ID:      "variables",
		Title:   "Template Variables",
		Entries: entries,
	}
}

func (ex *extractor) buildPatternSection() section {
	entries := make([]entry, 0, len(ex.matcherTypes)+1)
	types := append([]string(nil), ex.matcherTypes...)
	slices.Sort(types)
	for _, name := range types {
		e := entry{
			ID:   "pattern-" + name,
			Name: name,
			Kind: "pattern",
			Tags: []string{"pattern"},
		}
		switch name {
		case "string":
			e.Syntax = "<value>"
		case "glob":
			e.Syntax = `glob("<pattern>")`
		case "regex":
			e.Syntax = `regex("<pattern>")`
		}
		entries = append(entries, e)
	}
	return section{
		ID:      "patterns",
		Title:   "Pattern Helpers",
		Entries: entries,
	}
}

// trimCheatsheetProseTail removes trailing prose introducers ("…, e.g.:" / "…, e.g."),
// clause-ending punctuation (, ; ! ?), and a terminal sentence full stop. A trailing ':'
// is trimmed only when a ", e.g." suffix was removed, so headings like "Supported formats are:"
// stay intact. A final '.' is kept when it belongs to a known abbreviation (e.g. "i.e.").
func trimCheatsheetProseTail(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	beforeEg := s
	s = strings.TrimSpace(trailingCommaEgPhraseRE.ReplaceAllString(s, ""))
	strippedEg := s != beforeEg

	prev := ""
	for strings.TrimSpace(prev) != s {
		prev = s
		s = strings.TrimSpace(strings.TrimRight(s, ",;!? \t\f\v\r\u00a0"))
		if strippedEg {
			s = strings.TrimSpace(strings.TrimRight(s, ":"))
		}
	}
	return trimTrailingFullStop(strings.TrimSpace(s))
}

func trimTrailingFullStop(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, ".") {
		return s
	}
	lower := strings.ToLower(s)
	for _, suffix := range cheatsheetPreserveFinalPeriodSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return s
		}
	}
	return strings.TrimSpace(strings.TrimSuffix(s, "."))
}

func sanitizeCheatsheetLines(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = trimCheatsheetProseTail(line)
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func sanitizeCheatsheetArgs(args []arg) []arg {
	if len(args) == 0 {
		return args
	}
	out := slices.Clone(args)
	for i := range out {
		out[i].Description = trimCheatsheetProseTail(out[i].Description)
	}
	return out
}

func trimRedundantSyntaxHeading(lines []string) []string {
	const heading = "Syntax (within a rule do block):"
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == heading {
			continue
		}
		out = append(out, line)
	}
	return out
}

func (ex *extractor) buildControlSection() section {
	entries := make([]entry, 0, len(ex.controlDocs)+3)

	if defaultChecker, ok := ex.checkers["default"]; ok {
		e := ex.entryFromHelp("control", defaultChecker.Help.Command, defaultChecker.Help)
		e.ID = "control-default"
		e.Kind = "control"
		e.Phase = "pre/post"
		e.Tags = []string{"control", "pre/post"}
		entries = append(entries, e)
	}

	if lines, ok := ex.controlDocs["IfBlockCommand"]; ok {
		syntax := firstCodeLine(lines)
		description := sanitizeCheatsheetLines(normalizeControlDocLines(lines))
		var summary string
		var body []string
		if len(description) > 0 {
			summary = description[0]
			body = trimRedundantSyntaxHeading(description[1:])
		}
		entries = append(entries, controlEntry("control-inline-block", "if", summary, body, syntax))
	}

	if lines, ok := ex.controlDocs["IfElseBlockCommand"]; ok {
		syntax := firstCodeLine(lines)
		description := sanitizeCheatsheetLines(normalizeControlDocLines(lines))
		var summary string
		var body []string
		if len(description) > 0 {
			summary = description[0]
			body = trimRedundantSyntaxHeading(description[1:])
		}
		if hasBranchSyntax(syntax, "elif") {
			entries = append(entries, controlEntry("control-elif", "elif", summary, slices.Clone(body), branchSyntax(syntax, "elif")))
		}
		if hasBranchSyntax(syntax, "else") {
			entries = append(entries, controlEntry("control-else", "else", summary, slices.Clone(body), branchSyntax(syntax, "else")))
		}
	}
	return section{
		ID:          "control-flow",
		Title:       "Control Flow",
		Description: "Conditional branches inside a rule do-block (inherits the parent rule's phase).",
		Entries:     entries,
	}
}

func controlEntry(id, name, summary string, description []string, syntax string) entry {
	return entry{
		ID:          id,
		Name:        name,
		Kind:        "control",
		Summary:     summary,
		Description: description,
		Syntax:      syntax,
		Phase:       "pre/post",
		Tags:        []string{"control", "pre/post"},
	}
}

func (ex *extractor) entryFromHelp(kind string, name string, help helpData) entry {
	description, examples := splitExamples(name, help.Description)
	description = sanitizeCheatsheetLines(description)
	args := sanitizeCheatsheetArgs(help.Args)
	return entry{
		ID:          kind + "-" + name,
		Name:        name,
		Kind:        kind,
		Summary:     firstMeaningfulSummary(description),
		Description: description,
		Syntax:      inferSyntax(name, args),
		Examples:    examples,
		Args:        args,
	}
}

func splitExamples(name string, lines []string) (description []string, examples []string) {
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == name || strings.HasPrefix(line, name+" ") || strings.HasPrefix(line, name+"(") {
			examples = append(examples, line)
			continue
		}
		description = append(description, line)
	}
	return description, examples
}

func inferSyntax(name string, args []arg) string {
	if strings.HasPrefix(name, "$") {
		return inferVariableSyntax(name, args)
	}
	if len(args) == 0 {
		return name
	}
	parts := []string{name}
	for _, arg := range args {
		parts = append(parts, argPlaceholder(arg.Name))
	}
	return strings.Join(parts, " ")
}

func inferVariableSyntax(name string, args []arg) string {
	if len(args) == 0 {
		return name
	}
	required := make([]string, 0, len(args))
	optional := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg.Name, "[") && strings.HasSuffix(arg.Name, "]") {
			inner := strings.TrimSuffix(strings.TrimPrefix(arg.Name, "["), "]")
			optional = append(optional, "<"+inner+">")
			continue
		}
		required = append(required, "<"+arg.Name+">")
	}
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('(')
	sb.WriteString(strings.Join(required, ", "))
	for _, opt := range optional {
		sb.WriteString("[, ")
		sb.WriteString(opt)
		sb.WriteByte(']')
	}
	sb.WriteByte(')')
	return sb.String()
}

func argPlaceholder(name string) string {
	if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
		inner := strings.TrimSuffix(strings.TrimPrefix(name, "["), "]")
		return "[<" + inner + ">]"
	}
	return "<" + name + ">"
}

func (ex *extractor) inferMatcherPhase(validateExpr ast.Expr) string {
	return ex.phaseLabel(ex.inferPhaseExpr(validateExpr))
}

func (ex *extractor) inferCommandPhase(validateExpr ast.Expr) string {
	return ex.phaseLabel(ex.inferPhaseExpr(validateExpr))
}

func (ex *extractor) inferFieldPhase(validateExpr ast.Expr) string {
	return ex.phaseLabel(ex.inferPhaseExpr(validateExpr))
}

func (ex *extractor) inferSupportedActions(build *ast.FuncLit) []string {
	if build == nil || len(build.Body.List) == 0 {
		return nil
	}
	var actions []string
	ast.Inspect(build.Body, func(node ast.Node) bool {
		kv, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		name := identName(kv.Key)
		switch name {
		case "set", "add", "remove":
			actions = append(actions, name)
		}
		return true
	})
	slices.Sort(actions)
	return slices.Compact(actions)
}

func (ex *extractor) applyCommandAliases() {
	aliasTargets := map[string]string{}
	for key, item := range ex.commands {
		if item.Help.Command != "" && item.Help.Command != key {
			aliasTargets[key] = item.Help.Command
		}
	}
	for key, target := range aliasTargets {
		item := ex.commands[key]
		item.Help.Command = target
		ex.commands[key] = item
	}
}

func (ex *extractor) phaseFromExpr(expr ast.Expr) string {
	return ex.phaseLabel(ex.phaseMaskFromExpr(expr, nil))
}

func (ex *extractor) funcLitFromExpr(expr ast.Expr) *ast.FuncLit {
	switch n := expr.(type) {
	case *ast.FuncLit:
		return n
	case *ast.CallExpr:
		for _, arg := range n.Args {
			if lit, ok := arg.(*ast.FuncLit); ok {
				return lit
			}
		}
	}
	return nil
}

func (ex *extractor) stringValue(expr ast.Expr) (string, bool) {
	switch n := expr.(type) {
	case *ast.BasicLit:
		if n.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(n.Value)
		if err != nil {
			return "", false
		}
		return value, true
	case *ast.Ident:
		value, ok := ex.consts[n.Name]
		return value, ok
	}
	return "", false
}

func (ex *extractor) boolValue(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "true"
}

func identName(expr ast.Expr) string {
	switch n := expr.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.SelectorExpr:
		return n.Sel.Name
	}
	return ""
}

func sortedKeys[T any](m map[string]T) []string {
	keys := slices.Collect(maps.Keys(m))
	slices.Sort(keys)
	return keys
}

func mapsToSortedEntries(m map[string]*entry) []entry {
	keys := sortedKeys(m)
	out := make([]entry, 0, len(keys))
	for _, key := range keys {
		item := *m[key]
		slices.Sort(item.Aliases)
		out = append(out, item)
	}
	return out
}

func compact(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func firstMeaningfulSummary(lines []string) string {
	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "The template supports the following variables:") {
			continue
		}
		return line
	}
	return ""
}

func extractCommentLines(doc *ast.CommentGroup) []string {
	if doc == nil {
		return nil
	}
	raw := strings.Split(doc.Text(), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// isDocSyntaxLine matches indented godoc syntax examples such as "<on-expr> { <do...> }".
// Avoid treating prose (e.g. NOTE lines with `{`/`}` in backticks) as syntax.
func isDocSyntaxLine(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "<") && strings.Contains(line, "{") && strings.Contains(line, "}")
}

func firstCodeLine(lines []string) string {
	for _, line := range lines {
		if isDocSyntaxLine(line) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func nonCodeLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if isDocSyntaxLine(line) {
			continue
		}
		out = append(out, line)
	}
	return out
}

func normalizeControlDocLines(lines []string) []string {
	raw := nonCodeLines(lines)
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		out = append(out, stripLeadingGoTypeSentence(line))
	}
	return out
}

// stripLeadingGoTypeSentence turns "IfBlockCommand is an inline …" into "Inline …".
func stripLeadingGoTypeSentence(line string) string {
	line = strings.TrimSpace(line)
	spaceIdx := strings.IndexByte(line, ' ')
	if spaceIdx <= 0 {
		return line
	}
	firstTok := line[:spaceIdx]
	r0, _ := utf8.DecodeRuneInString(firstTok)
	if !unicode.IsUpper(r0) {
		return line
	}
	rest := strings.TrimSpace(line[spaceIdx+1:])
	for _, prefix := range []string{"is a ", "is an "} {
		after, ok := strings.CutPrefix(rest, prefix)
		if !ok {
			continue
		}
		after = strings.TrimSpace(after)
		if after == "" {
			return line
		}
		r, size := utf8.DecodeRuneInString(after)
		if size == 0 {
			return line
		}
		return string(unicode.ToUpper(r)) + after[size:]
	}
	return line
}

func controlFlowEntryOrder(id string) int {
	switch id {
	case "control-inline-block":
		return 0
	case "control-elif":
		return 1
	case "control-else":
		return 2
	case "control-default":
		return 3
	default:
		return 99
	}
}

const (
	phaseMaskUnknown = -1
	phaseMaskNone    = 0
	phaseMaskPre     = 1 << iota
	phaseMaskPost
)

func (ex *extractor) inferPhaseExpr(expr ast.Expr) int {
	switch n := expr.(type) {
	case *ast.Ident:
		return ex.inferNamedFunctionPhase(n.Name)
	case *ast.FuncLit:
		return ex.inferFuncLitPhase(n)
	case *ast.CallExpr:
		return ex.phaseMaskFromCall(n, nil)
	default:
		return phaseMaskUnknown
	}
}

func (ex *extractor) inferNamedFunctionPhase(name string) int {
	if name == "" {
		return phaseMaskUnknown
	}
	if special := ex.phaseMaskFromFunctionName(name); special != phaseMaskUnknown {
		return special
	}
	fn, ok := ex.funcs[name]
	if !ok {
		return phaseMaskUnknown
	}
	return ex.inferBlockPhase(fn.Body)
}

func (ex *extractor) inferFuncLitPhase(lit *ast.FuncLit) int {
	if lit == nil {
		return phaseMaskUnknown
	}
	return ex.inferBlockPhase(lit.Body)
}

func (ex *extractor) inferBlockPhase(block *ast.BlockStmt) int {
	if block == nil {
		return phaseMaskUnknown
	}
	env := map[string]int{
		"phase": phaseMaskNone,
	}
	mask := phaseMaskNone
	for _, stmt := range block.List {
		mask |= ex.inferStmtPhase(stmt, env)
	}
	if env["phase"] != phaseMaskNone {
		mask |= env["phase"]
	}
	if mask == phaseMaskNone {
		return phaseMaskUnknown
	}
	return mask
}

func (ex *extractor) inferStmtPhase(stmt ast.Stmt, env map[string]int) int {
	switch n := stmt.(type) {
	case *ast.AssignStmt:
		return ex.inferAssignPhase(n, env)
	case *ast.ReturnStmt:
		mask := phaseMaskNone
		for _, result := range n.Results {
			mask |= ex.phaseMaskFromExpr(result, env)
		}
		return mask
	case *ast.IfStmt:
		mask := ex.inferBlockPhase(n.Body)
		if n.Else != nil {
			switch elseNode := n.Else.(type) {
			case *ast.BlockStmt:
				mask |= ex.inferBlockPhase(elseNode)
			case *ast.IfStmt:
				mask |= ex.inferStmtPhase(elseNode, maps.Clone(env))
			}
		}
		return mask
	case *ast.SwitchStmt:
		mask := phaseMaskNone
		for _, stmt := range n.Body.List {
			clause, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, inner := range clause.Body {
				mask |= ex.inferStmtPhase(inner, maps.Clone(env))
			}
		}
		return mask
	}
	return phaseMaskNone
}

func (ex *extractor) inferAssignPhase(assign *ast.AssignStmt, env map[string]int) int {
	mask := phaseMaskNone
	switch assign.Tok {
	case token.ASSIGN, token.DEFINE:
		for i, lhs := range assign.Lhs {
			name := identName(lhs)
			if name == "" {
				continue
			}
			if i >= len(assign.Rhs) {
				env[name] = phaseMaskNone
				continue
			}
			valueMask := ex.phaseMaskFromExpr(assign.Rhs[i], env)
			env[name] = valueMask
			if name == "phase" {
				mask |= valueMask
			}
		}
	case token.OR_ASSIGN:
		if len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
			name := identName(assign.Lhs[0])
			valueMask := ex.phaseMaskFromExpr(assign.Rhs[0], env)
			env[name] |= valueMask
			if name == "phase" {
				mask |= env[name]
			}
		}
	}
	return mask
}

func (ex *extractor) phaseMaskFromExpr(expr ast.Expr, env map[string]int) int {
	switch n := expr.(type) {
	case *ast.Ident:
		switch n.Name {
		case "nil", "true", "false":
			return phaseMaskNone
		case "PhasePre":
			return phaseMaskPre
		case "PhasePost":
			return phaseMaskPost
		case "PhaseNone", "phase":
			if env != nil {
				if mask, ok := env[n.Name]; ok {
					return mask
				}
			}
			return phaseMaskNone
		default:
			if env != nil {
				if mask, ok := env[n.Name]; ok {
					return mask
				}
			}
			return ex.inferNamedFunctionPhase(n.Name)
		}
	case *ast.BinaryExpr:
		if n.Op == token.OR || n.Op == token.ADD {
			return ex.phaseMaskFromExpr(n.X, env) | ex.phaseMaskFromExpr(n.Y, env)
		}
	case *ast.CallExpr:
		return ex.phaseMaskFromCall(n, env)
	}
	return phaseMaskUnknown
}

func (ex *extractor) phaseMaskFromCall(call *ast.CallExpr, env map[string]int) int {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		name := fn.Name
		if name == "validateModField" {
			return ex.unionFieldPhase()
		}
		return ex.inferNamedFunctionPhase(name)
	case *ast.SelectorExpr:
		if fn.Sel.Name == "validate" {
			if ident, ok := fn.X.(*ast.Ident); ok && ident.Name == "setField" {
				return ex.unionFieldPhase()
			}
		}
	}
	return phaseMaskUnknown
}

func (ex *extractor) unionFieldPhase() int {
	mask := phaseMaskNone
	for _, field := range ex.fields {
		mask |= ex.inferFieldPhaseMask(field.Validate)
	}
	return mask
}

func (ex *extractor) inferFieldPhaseMask(validateExpr ast.Expr) int {
	return ex.inferPhaseExpr(validateExpr)
}

func (ex *extractor) phaseMaskFromFunctionName(name string) int {
	switch {
	case strings.Contains(name, "Post"):
		return phaseMaskPost
	case strings.Contains(name, "Pre"):
		return phaseMaskPre
	case name == "validateTemplate":
		return phaseMaskPre | phaseMaskPost
	case name == "ValidateVars", name == "ExpandVars":
		return phaseMaskPre | phaseMaskPost
	case name == "validateLevel", name == "validateURL", name == "validateCIDR", name == "validateMethod", name == "validateStatusRange", name == "validateStatusCode", name == "validateSingleMatcher", name == "toKVOptionalVMatcher", name == "validateURLPath", name == "validateURLPathMatcher", name == "validateFSPath", name == "validateUserBCryptPassword":
		return phaseMaskNone
	case name == "validateModField":
		return ex.unionFieldPhase()
	}
	return phaseMaskUnknown
}

func (ex *extractor) phaseLabel(mask int) string {
	if mask == phaseMaskUnknown {
		return ""
	}
	switch {
	case mask&phaseMaskPre != 0 && mask&phaseMaskPost != 0:
		return "pre/post"
	case mask&phaseMaskPost != 0:
		return "post"
	default:
		return "pre"
	}
}

func hasBranchSyntax(syntax, keyword string) bool {
	switch keyword {
	case "elif":
		return strings.Contains(syntax, "} elif ")
	case "else":
		return strings.Contains(syntax, " else {")
	default:
		return strings.Contains(syntax, keyword)
	}
}

func branchSyntax(syntax, keyword string) string {
	index := strings.Index(syntax, " "+keyword+" ")
	if index == -1 {
		return syntax
	}
	return strings.TrimSpace(syntax[index+1:])
}

package main

import (
	"path/filepath"
	"testing"
)

func TestBuildVariableSectionIncludesDescriptions(t *testing.T) {
	rulesDir := filepath.Join("..", "..", "internal", "route", "rules")
	ex, err := parseRulesDir(rulesDir)
	if err != nil {
		t.Fatalf("parseRulesDir: %v", err)
	}

	section := ex.buildVariableSection()
	if section.ID != "variables" {
		t.Fatalf("unexpected section id %q", section.ID)
	}
	if len(section.Entries) == 0 {
		t.Fatal("expected variable entries")
	}

	entries := map[string]entry{}
	for _, item := range section.Entries {
		entries[item.Name] = item
		if item.Summary == "" {
			t.Fatalf("variable %s is missing summary", item.Name)
		}
	}

	reqMethod, ok := entries["$req_method"]
	if !ok {
		t.Fatal("missing $req_method entry")
	}
	if reqMethod.Summary != "Inbound HTTP method (verb)" {
		t.Fatalf("unexpected $req_method summary %q", reqMethod.Summary)
	}

	header, ok := entries["$header"]
	if !ok {
		t.Fatal("missing $header entry")
	}
	if got := header.Syntax; got != "$header(<name>[, <index>])" {
		t.Fatalf("unexpected $header syntax %q", got)
	}
	if len(header.Args) != 2 {
		t.Fatalf("expected 2 args for $header, got %d", len(header.Args))
	}
	if got := header.Args[0].Description; got != "Exact request header name" {
		t.Fatalf("unexpected $header name description %q", got)
	}

	postForm, ok := entries["$postform"]
	if !ok {
		t.Fatal("missing $postform entry")
	}
	if postForm.Summary == "" || len(postForm.Description) == 0 {
		t.Fatal("$postform should include descriptive text")
	}
}

func TestBuildCommandSectionUsesMeaningfulSummary(t *testing.T) {
	rulesDir := filepath.Join("..", "..", "internal", "route", "rules")
	ex, err := parseRulesDir(rulesDir)
	if err != nil {
		t.Fatalf("parseRulesDir: %v", err)
	}

	section := ex.buildCommandSection()
	entries := map[string]entry{}
	for _, item := range section.Entries {
		entries[item.Name] = item
	}

	logEntry, ok := entries["log"]
	if !ok {
		t.Fatal("missing log entry")
	}
	if logEntry.Summary != "Write a log line from a template" {
		t.Fatalf("unexpected log summary %q", logEntry.Summary)
	}

	notifyEntry, ok := entries["notify"]
	if !ok {
		t.Fatal("missing notify entry")
	}
	if notifyEntry.Summary != "Send a notification from templates" {
		t.Fatalf("unexpected notify summary %q", notifyEntry.Summary)
	}
	if notifyEntry.Syntax != "notify <level> <provider> <title> <body>" {
		t.Fatalf("unexpected notify syntax %q", notifyEntry.Syntax)
	}
	if len(notifyEntry.Args) != 4 {
		t.Fatalf("expected 4 args for notify, got %d", len(notifyEntry.Args))
	}
	if notifyEntry.Args[0].Name != "level" || notifyEntry.Args[3].Name != "body" {
		t.Fatalf("unexpected notify arg order: %#v", notifyEntry.Args)
	}
}

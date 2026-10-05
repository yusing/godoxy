package main

import (
	"bytes"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yusing/godoxy/internal/autocert"
)

func TestProviderNames(t *testing.T) {
	names := providerNames()
	if !slices.IsSorted(names) || len(slices.Compact(slices.Clone(names))) != len(names) {
		t.Fatal("provider IDs must be sorted and unique")
	}
	for _, name := range []string{"local", "custom", "spaceship", "dnsupdate", "rfc2136"} {
		if !slices.Contains(names, name) {
			t.Errorf("missing supported provider %s", name)
		}
	}
	for _, name := range []string{"pseudo", "route53", "googledomains"} {
		if slices.Contains(names, name) {
			t.Errorf("unsupported picker option %s", name)
		}
	}
	// Keep the catalogue tied to registration, not another manually maintained list.
	autocert.Providers["new-provider"] = autocert.Generator{}
	t.Cleanup(func() { delete(autocert.Providers, "new-provider") })
	if !slices.Contains(providerNames(), "new-provider") {
		t.Fatal("new registrations must be generated automatically")
	}
}

func TestReplaceTable(t *testing.T) {
	current := []byte("intro\n" + startMarker + "\nold table\n" + endMarker + "\nexamples\n")
	want := "intro\n" + renderTable(providerNames()) + "examples\n"
	got, err := replaceTable(current, renderTable(providerNames()))
	if err != nil || string(got) != want {
		t.Fatalf("table replacement must preserve surrounding documentation: %v", err)
	}
	for _, invalid := range []string{"", endMarker + startMarker, startMarker + startMarker + endMarker} {
		if _, err := replaceTable([]byte(invalid), "table"); err == nil {
			t.Fatal("missing, reversed, or duplicate markers must fail")
		}
	}
}

func TestGenerateAndCheckDrift(t *testing.T) {
	root := t.TempDir()
	paths := append([]string{typePath}, docPaths...)
	for _, path := range paths {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if path != typePath {
			if err := os.WriteFile(fullPath, []byte(startMarker+"\n"+endMarker+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Use the actual consuming schemas, not a second fixture provider catalogue.
	for _, name := range []string{"autocert.schema.json", "config.schema.json"} {
		path := "webui/src/types/godoxy/" + name
		data, err := os.ReadFile(filepath.Join("../..", path))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := generate(root, true); err == nil || !strings.Contains(err.Error(), typePath) {
		t.Fatal("check must report missing provider type")
	}
	if _, err := os.Stat(filepath.Join(root, typePath)); !os.IsNotExist(err) {
		t.Fatal("check must not create missing files")
	}
	if err := generate(root, false); err != nil {
		t.Fatal(err)
	}
	if err := generate(root, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			fullPath := filepath.Join(root, path)
			current, err := os.ReadFile(fullPath)
			if err != nil {
				t.Fatal(err)
			}
			stale := bytes.Replace(current, []byte("spaceship"), []byte("stale-provider"), 1)
			if bytes.Equal(current, stale) {
				t.Fatal("fixture must contain Spaceship")
			}
			if err := os.WriteFile(fullPath, stale, 0644); err != nil {
				t.Fatal(err)
			}
			if err := generate(root, true); err == nil || !strings.Contains(err.Error(), path) {
				t.Fatal("guard must identify the stale artifact")
			}
			got, err := os.ReadFile(fullPath)
			if err != nil || !bytes.Equal(got, stale) {
				t.Fatal("check modified a stale artifact")
			}
			if err := generate(root, false); err != nil {
				t.Fatal(err)
			}
			got, err = os.ReadFile(fullPath)
			if err != nil || !bytes.Equal(got, current) {
				t.Fatal("generation must be deterministic")
			}
		})
	}
	// A missing provider in either primary or extra schemas must trip the guard.
	path := filepath.Join(root, "webui/src/types/godoxy/config.schema.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc schema
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AutocertConfig", "AutocertExtra"} {
		t.Run(name, func(t *testing.T) {
			original := doc.Definitions[name]
			doc.Definitions[name] = schema{Properties: map[string]schema{"provider": {Const: "local"}}}
			stale, err := json.Marshal(&doc)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, stale, 0644); err != nil {
				t.Fatal(err)
			}
			if err := checkSchemas(root, providerNames()); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatal("guard must cover primary and extra provider schemas")
			}
			doc.Definitions[name] = original
		})
	}
}

func TestSchemaProvidersRejectsUnrestrictedStrings(t *testing.T) {
	if _, err := schemaProviders(schema{Properties: map[string]schema{"provider": {}}}, nil); err == nil {
		t.Fatal("provider: string must fail the guard")
	}
	if _, err := schemaProviders(schema{Ref: "#/definitions/Missing"}, nil); err == nil {
		t.Fatal("missing schema references must fail the guard")
	}
}

func TestOptionSchemaGuardDetectsMissingAndIncorrectFields(t *testing.T) {
	providerNames()
	data, err := os.ReadFile("../../webui/src/types/godoxy/autocert.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc schema
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	root := doc.Definitions["AutocertConfigWithoutExtra"]
	if err := checkProviderOptionSchemas(root, doc.Definitions); err != nil {
		t.Fatal(err)
	}
	options := doc.Definitions["SpaceshipOptions"].Properties["options"]
	original := options.Properties["api_secret"]
	delete(options.Properties, "api_secret")
	if err := checkProviderOptionSchemas(root, doc.Definitions); err == nil {
		t.Fatal("guard must detect a missing concrete option field")
	}
	options.Properties["api_secret"] = original
	options.Properties["api_secret"] = schema{Type: []byte(`"number"`)}
	if err := checkProviderOptionSchemas(root, doc.Definitions); err == nil {
		t.Fatal("guard must detect an incorrect option field type")
	}
}

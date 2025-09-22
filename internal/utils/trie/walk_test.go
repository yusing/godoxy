package trie_test

import (
	"maps"
	"slices"
	"testing"

	. "github.com/yusing/godoxy/internal/utils/trie"
)

// Test data for trie tests
var (
	testData = map[string]any{
		"routes.route1":                      new(int),
		"routes.route2":                      new(int),
		"routes.route3":                      new(int),
		"system.cpu_average":                 new(int),
		"system.mem.used":                    new(int),
		"system.mem.percentage_used":         new(int),
		"system.disks.disk0.used":            new(int),
		"system.disks.disk0.percentage_used": new(int),
		"system.disks.disk1.used":            new(int),
		"system.disks.disk1.percentage_used": new(int),
	}

	testWalkDisksWants = []string{
		"system.disks.disk0.used",
		"system.disks.disk0.percentage_used",
		"system.disks.disk1.used",
		"system.disks.disk1.percentage_used",
	}
	testWalkDisksUsedWants = []string{
		"system.disks.disk0.used",
		"system.disks.disk1.used",
	}
	testUsedWants = []string{
		"system.mem.used",
		"system.disks.disk0.used",
		"system.disks.disk1.used",
	}
)

// Helper functions
func keys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}

func keysEqual(m map[string]any, want []string) bool {
	slices.Sort(want)
	return slices.Equal(keys(m), want)
}

func TestWalkAll(t *testing.T) {
	trie := NewTrie()
	for key, series := range testData {
		trie.Store(NewKey(key), series)
	}

	walked := maps.Collect(trie.Walk)
	for k, v := range testData {
		if _, ok := walked[k]; !ok {
			t.Fatalf("expected key %s not found", k)
		}
		if v != walked[k] {
			t.Fatalf("key %s expected %v, got %v", k, v, walked[k])
		}
	}
}

func TestWalk(t *testing.T) {
	trie := NewTrie()
	for key, series := range testData {
		trie.Store(NewKey(key), series)
	}

	tests := []struct {
		query     string
		want      []string
		wantEmpty bool
	}{
		{"system.disks.*.used", testWalkDisksUsedWants, false},
		{"system.*.*.used", testWalkDisksUsedWants, false},
		{"*.disks.*.used", testWalkDisksUsedWants, false},
		{"*.*.*.used", testWalkDisksUsedWants, false},
		{"system.disks.**", testWalkDisksWants, false}, // note: original code uses '*' not '**'
		{"system.disks", nil, true},
		{"**.used", testUsedWants, false},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			got := maps.Collect(trie.Query(NewKey(tc.query)))
			if tc.wantEmpty {
				if len(got) != 0 {
					t.Fatalf("expected empty, got %v", keys(got))
				}
				return
			}
			if !keysEqual(got, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, keys(got))
			}
			for _, k := range tc.want {
				want, ok := testData[k]
				if !ok {
					t.Fatalf("expected key %s not found", k)
				}
				if got[k] != want {
					t.Fatalf("key %s expected %v, got %v", k, want, got[k])
				}
			}
		})
	}
}

package docker

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/types"
)

func TestParseLabelsIgnoresNonProxyAndRejectsInvalidRoot(t *testing.T) {
	parsed, err := ParseLabels(map[string]string{
		"other.label": "value",
		"proxy":       "invalid",
	})

	require.ErrorIs(t, err, ErrInvalidLabel)
	require.Empty(t, parsed)
}

func TestParseLabelsPromotesEmptyStringIntoNestedObject(t *testing.T) {
	parsed, err := ParseLabels(map[string]string{
		"proxy.a.b":   "",
		"proxy.a.b.c": "value",
	})

	require.NoError(t, err)
	require.Equal(t, types.LabelMap{
		"a": types.LabelMap{
			"b": types.LabelMap{
				"c": "value",
			},
		},
	}, parsed)
}

func TestParseLabelsMergesObjectIntoExistingMap(t *testing.T) {
	parsed, err := ParseLabels(map[string]string{
		"proxy.a.b":   "c: generic\nd: merged",
		"proxy.a.b.c": "specific",
	})

	require.NoError(t, err)
	require.Equal(t, types.LabelMap{
		"a": types.LabelMap{
			"b": types.LabelMap{
				"c": "specific",
				"d": "merged",
			},
		},
	}, parsed)
}

func TestParseLabelsRejectsInvalidObjectMergeValue(t *testing.T) {
	parsed, err := ParseLabels(map[string]string{
		"proxy.a.b":   "- invalid",
		"proxy.a.b.c": "specific",
	})

	require.ErrorContains(t, err, "proxy.a.b.c")
	require.ErrorContains(t, err, "expect mapping, got string")
	require.Equal(t, types.LabelMap{
		"a": types.LabelMap{
			"b": "- invalid",
		},
	}, parsed)
}

func TestParseLabelsRejectsSpecificFieldOverrideOfNestedObjectField(t *testing.T) {
	parsed, err := ParseLabels(map[string]string{
		"proxy.a.b":   "c:\n  nested: value",
		"proxy.a.b.c": "specific",
	})

	require.ErrorContains(t, err, "proxy.a.b.c")
	require.ErrorContains(t, err, "expect mapping, got string")
	require.Equal(t, types.LabelMap{
		"a": types.LabelMap{
			"b": types.LabelMap{
				"c": types.LabelMap{
					"nested": "value",
				},
			},
		},
	}, parsed)
}

func TestParseLabelsMergesIntoExistingNestedMap(t *testing.T) {
	parsed, err := ParseLabels(map[string]string{
		"proxy.a.b":   "c:\n  nested:\n    allow: true",
		"proxy.a.b.c": "nested:\n  deny: true",
	})

	require.NoError(t, err)
	require.Equal(t, types.LabelMap{
		"a": types.LabelMap{
			"b": types.LabelMap{
				"c": types.LabelMap{
					"nested": types.LabelMap{
						"allow": true,
						"deny":  true,
					},
				},
			},
		},
	}, parsed)
}

func TestParseLabelsRejectsInvalidNestedObjectMergeValue(t *testing.T) {
	parsed, err := ParseLabels(map[string]string{
		"proxy.a.b":   "c:\n  nested: value",
		"proxy.a.b.c": "- invalid",
	})

	require.ErrorContains(t, err, "proxy.a.b.c")
	require.ErrorContains(t, err, "expect mapping, got string")
	require.Equal(t, types.LabelMap{
		"a": types.LabelMap{
			"b": types.LabelMap{
				"c": types.LabelMap{
					"nested": "value",
				},
			},
		},
	}, parsed)
}

func TestParseLabelsRejectsConflictingNestedObjectMerge(t *testing.T) {
	parsed, err := ParseLabels(map[string]string{
		"proxy.a.b":   "c:\n  nested:\n    allow: true",
		"proxy.a.b.c": "nested: blocked",
	})

	require.ErrorContains(t, err, "proxy.a.b.c")
	require.ErrorContains(t, err, "expect mapping, got string")
	require.Equal(t, types.LabelMap{
		"a": types.LabelMap{
			"b": types.LabelMap{
				"c": types.LabelMap{
					"nested": types.LabelMap{
						"allow": true,
					},
				},
			},
		},
	}, parsed)
}

func TestParseLabelsRejectsNestedFieldInsideScalarObjectMember(t *testing.T) {
	parsed, err := ParseLabels(map[string]string{
		"proxy.a.b":     "c: 1",
		"proxy.a.b.c.d": "value",
	})

	require.ErrorContains(t, err, "proxy.a.b.c.d")
	require.ErrorContains(t, err, "expect mapping, got uint64")
	require.Equal(t, types.LabelMap{
		"a": types.LabelMap{
			"b": types.LabelMap{
				"c": uint64(1),
			},
		},
	}, parsed)
}

func TestParseLabelObject(t *testing.T) {
	t.Run("empty string becomes empty map", func(t *testing.T) {
		parsed, ok := parseLabelObject("")
		require.True(t, ok)
		require.Empty(t, parsed)
	})

	t.Run("yaml object parses", func(t *testing.T) {
		parsed, ok := parseLabelObject("nested:\n\tvalue: true")
		require.True(t, ok)
		require.Equal(t, types.LabelMap{
			"nested": types.LabelMap{
				"value": true,
			},
		}, parsed)
	})

	t.Run("non-object yaml is rejected", func(t *testing.T) {
		parsed, ok := parseLabelObject("- item")
		require.False(t, ok)
		require.Nil(t, parsed)
	})
}

func TestMergeLabelMaps(t *testing.T) {
	t.Run("recursively merges nested maps and preserves specific scalar overrides", func(t *testing.T) {
		dst := types.LabelMap{
			"allowed_groups": []any{"specific"},
			"bypass": types.LabelMap{
				"path": "/private",
			},
		}
		src := types.LabelMap{
			"allowed_groups": []any{"generic"},
			"bypass": types.LabelMap{
				"methods": "GET",
			},
			"priority": 5,
		}

		err := mergeLabelMaps(dst, src)
		require.NoError(t, err)
		require.Equal(t, types.LabelMap{
			"allowed_groups": []any{"specific"},
			"bypass": types.LabelMap{
				"path":    "/private",
				"methods": "GET",
			},
			"priority": 5,
		}, dst)
	})

	t.Run("rejects map receiving scalar", func(t *testing.T) {
		err := mergeLabelMaps(types.LabelMap{
			"bypass": types.LabelMap{"path": "/private"},
		}, types.LabelMap{
			"bypass": "skip",
		})

		require.ErrorContains(t, err, "expect mapping")
	})

	t.Run("rejects scalar receiving map", func(t *testing.T) {
		err := mergeLabelMaps(types.LabelMap{
			"bypass": "skip",
		}, types.LabelMap{
			"bypass": types.LabelMap{"path": "/private"},
		})

		require.ErrorContains(t, err, "cannot merge mapping into existing scalar")
	})

	t.Run("rejects nested recursive map conflicts", func(t *testing.T) {
		err := mergeLabelMaps(types.LabelMap{
			"outer": types.LabelMap{
				"nested": types.LabelMap{"allow": true},
			},
		}, types.LabelMap{
			"outer": types.LabelMap{
				"nested": "blocked",
			},
		})

		require.ErrorContains(t, err, "expect mapping")
	})
}

func TestCompareLabelKeys(t *testing.T) {
	require.Less(t, compareLabelKeys("proxy.a", "proxy.a.b"), 0)
	require.Less(t, compareLabelKeys("proxy.a.a", "proxy.a.b"), 0)
	require.Greater(t, compareLabelKeys("proxy.a.c", "proxy.a.b"), 0)
}

func TestFlattenMapAny(t *testing.T) {
	dest := make(map[string]string)

	flattenMapAny("", map[any]any{
		"nested": map[any]any{
			"string": "value",
			"int":    7,
			"bool":   true,
			"float":  1.5,
			9:        "numeric-key",
			"map": map[string]any{
				"child": "value",
			},
		},
		"list": []int{1, 2},
	}, dest)

	require.Equal(t, map[string]string{
		"nested.string":    "value",
		"nested.int":       "7",
		"nested.bool":      "true",
		"nested.float":     "1.5",
		"nested.9":         "numeric-key",
		"nested.map.child": "value",
		"list":             "[1 2]",
	}, dest)
}

func TestFlattenMap(t *testing.T) {
	dest := make(map[string]string)

	flattenMap("", map[string]any{
		"nested": map[string]any{
			"string": "value",
			"mapany": map[any]any{
				"child": "nested-value",
			},
			"int":   7,
			"bool":  true,
			"float": 1.5,
		},
		"list": []int{1, 2},
	}, dest)

	require.Equal(t, map[string]string{
		"nested.string":       "value",
		"nested.mapany.child": "nested-value",
		"nested.int":          "7",
		"nested.bool":         "true",
		"nested.float":        "1.5",
		"list":                "[1 2]",
	}, dest)
}

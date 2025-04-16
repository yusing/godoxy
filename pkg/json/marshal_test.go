package json_test

import (
	stdJSON "encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/stretchr/testify/require"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	. "github.com/yusing/go-proxy/internal/utils/testing"
	. "github.com/yusing/go-proxy/pkg/json"
)

type testStruct struct {
	Name  string  `json:"name"`
	Age   int     `json:"age"`
	Score float64 `json:"score"`
	Empty *struct {
		Value string `json:"value,omitempty"`
	} `json:"empty,omitempty"`
}

type stringer struct {
	testStruct
}

func (s stringer) String() string {
	return s.Name
}

type customMarshaler struct {
	Value string
}

func (cm customMarshaler) MarshalJSONTo(buf []byte) []byte {
	return append(buf, []byte(`{"custom":"`+cm.Value+`"}`)...)
}

type jsonMarshaler struct {
	Value string
}

func (jm jsonMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`{"json_marshaler":"` + jm.Value + `"}`), nil
}

type withJSONTag struct {
	Value string `json:"value"`
}

type withJSONOmitEmpty struct {
	Value string `json:"value,omitempty"`
}

type withJSONStringTag struct {
	Value int64 `json:"value,string"`
}

type withJSONOmit struct {
	Value string `json:"-"`
}

type withJSONByteSize struct {
	Value uint64 `json:"value,byte_size"`
}

type withJSONUnixTime struct {
	Value int64 `json:"value,unix_time"`
}

type primitiveWithMarshaler int

func (p primitiveWithMarshaler) MarshalJSONTo(buf []byte) []byte {
	return fmt.Appendf(buf, `%q`, strconv.Itoa(int(p)))
}

type withTagUseMarshaler struct {
	Value primitiveWithMarshaler `json:"value,use_marshaler"`
}

type Anonymous struct {
	Value  string `json:"value"`
	Value2 int    `json:"value2"`
}

type withAnonymous struct {
	Anonymous
}

type withPointerAnonymous struct {
	*Anonymous
}

type selfReferencing struct {
	Self *selfReferencing `json:"self"`
}

func TestMarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "string",
			input:    "test",
			expected: `"test"`,
		},
		{
			name:     "bool_true",
			input:    true,
			expected: `true`,
		},
		{
			name:     "bool_false",
			input:    false,
			expected: `false`,
		},
		{
			name:     "int",
			input:    42,
			expected: `42`,
		},
		{
			name:     "uint",
			input:    uint(42),
			expected: `42`,
		},
		{
			name:     "float",
			input:    3.14,
			expected: `3.14`,
		},
		{
			name:     "slice",
			input:    []int{1, 2, 3},
			expected: `[1,2,3]`,
		},
		{
			name:     "array",
			input:    [3]int{4, 5, 6},
			expected: `[4,5,6]`,
		},
		{
			name:     "slice_of_struct",
			input:    []testStruct{{Name: "John", Age: 30, Score: 8.5}, {Name: "Jane", Age: 25, Score: 9.5}},
			expected: `[{"name":"John","age":30,"score":8.5},{"name":"Jane","age":25,"score":9.5}]`,
		},
		{
			name:     "slice_of_struct_pointer",
			input:    []*testStruct{{Name: "John", Age: 30, Score: 8.5}, {Name: "Jane", Age: 25, Score: 9.5}},
			expected: `[{"name":"John","age":30,"score":8.5},{"name":"Jane","age":25,"score":9.5}]`,
		},
		{
			name:     "slice_of_map",
			input:    []map[string]any{{"key1": "value1"}, {"key2": "value2"}},
			expected: `[{"key1":"value1"},{"key2":"value2"}]`,
		},
		{
			name:     "struct",
			input:    testStruct{Name: "John", Age: 30, Score: 8.5},
			expected: `{"name":"John","age":30,"score":8.5}`,
		},
		{
			name:     "struct_pointer",
			input:    &testStruct{Name: "Jane", Age: 25, Score: 9.5},
			expected: `{"name":"Jane","age":25,"score":9.5}`,
		},
		{
			name:     "byte_slice",
			input:    []byte("test"),
			expected: `"dGVzdA=="`,
		},
		{
			name:     "custom_marshaler",
			input:    customMarshaler{Value: "test"},
			expected: `{"custom":"test"}`,
		},
		{
			name:     "custom_marshaler_pointer",
			input:    &customMarshaler{Value: "test"},
			expected: `{"custom":"test"}`,
		},
		{
			name:     "json_marshaler",
			input:    jsonMarshaler{Value: "test"},
			expected: `{"json_marshaler":"test"}`,
		},
		{
			name:     "json_marshaler_pointer",
			input:    &jsonMarshaler{Value: "test"},
			expected: `{"json_marshaler":"test"}`,
		},
		{
			name:     "stringer",
			input:    stringer{testStruct: testStruct{Name: "Bob", Age: 20, Score: 9.5}},
			expected: `"Bob"`,
		},
		{
			name:     "stringer_pointer",
			input:    &stringer{testStruct: testStruct{Name: "Bob", Age: 20, Score: 9.5}},
			expected: `"Bob"`,
		},
		{
			name:     "with_json_tag",
			input:    withJSONTag{Value: "test"},
			expected: `{"value":"test"}`,
		},
		{
			name:     "with_json_tag_pointer",
			input:    &withJSONTag{Value: "test"},
			expected: `{"value":"test"}`,
		},
		{
			name:     "with_json_omit_empty",
			input:    withJSONOmitEmpty{Value: "test"},
			expected: `{"value":"test"}`,
		},
		{
			name:     "with_json_omit_empty_pointer",
			input:    &withJSONOmitEmpty{Value: "test"},
			expected: `{"value":"test"}`,
		},
		{
			name:     "with_json_omit_empty_empty",
			input:    withJSONOmitEmpty{},
			expected: `{}`,
		},
		{
			name:     "with_json_omit_empty_pointer_empty",
			input:    &withJSONOmitEmpty{},
			expected: `{}`,
		},
		{
			name:     "with_json_omit",
			input:    withJSONOmit{Value: "test"},
			expected: `{}`,
		},
		{
			name:     "with_json_omit_pointer",
			input:    &withJSONOmit{Value: "test"},
			expected: `{}`,
		},
		{
			name:     "with_json_string_tag",
			input:    withJSONStringTag{Value: 1234567890},
			expected: `{"value":"1234567890"}`,
		},
		{
			name:     "with_json_string_tag_pointer",
			input:    &withJSONStringTag{Value: 1234567890},
			expected: `{"value":"1234567890"}`,
		},
		{
			name:     "with_json_byte_size",
			input:    withJSONByteSize{Value: 1024},
			expected: fmt.Sprintf(`{"value":"%s"}`, strutils.FormatByteSize(1024)),
		},
		{
			name:     "with_json_byte_size_pointer",
			input:    &withJSONByteSize{Value: 1024},
			expected: fmt.Sprintf(`{"value":"%s"}`, strutils.FormatByteSize(1024)),
		},
		{
			name:     "with_json_unix_time",
			input:    withJSONUnixTime{Value: 1713033600},
			expected: fmt.Sprintf(`{"value":"%s"}`, strutils.FormatUnixTime(1713033600)),
		},
		{
			name:     "with_json_unix_time_pointer",
			input:    &withJSONUnixTime{Value: 1713033600},
			expected: fmt.Sprintf(`{"value":"%s"}`, strutils.FormatUnixTime(1713033600)),
		},
		{
			name:     "with_tag_use_marshaler",
			input:    withTagUseMarshaler{Value: primitiveWithMarshaler(42)},
			expected: `{"value":"42"}`,
		},
		{
			name:     "with_tag_use_marshaler_pointer",
			input:    &withTagUseMarshaler{Value: primitiveWithMarshaler(42)},
			expected: `{"value":"42"}`,
		},
		{
			name:     "with_anonymous",
			input:    withAnonymous{Anonymous: Anonymous{Value: "test", Value2: 1}},
			expected: `{"value":"test","value2":1}`,
		},
		{
			name:     "with_anonymous_pointer",
			input:    &withAnonymous{Anonymous: Anonymous{Value: "test", Value2: 1}},
			expected: `{"value":"test","value2":1}`,
		},
		{
			name:     "with_pointer_anonymous",
			input:    &withPointerAnonymous{Anonymous: &Anonymous{Value: "test", Value2: 1}},
			expected: `{"value":"test","value2":1}`,
		},
		{
			name:     "with_pointer_anonymous_nil",
			input:    &withPointerAnonymous{Anonymous: nil},
			expected: `{}`,
		},
		{
			// NOTE: not fixing this until needed
			// GoDoxy does not have any type with exported self-referencing fields
			name: "self_referencing",
			input: func() *selfReferencing {
				s := &selfReferencing{}
				s.Self = s
				return s
			}(),
			expected: `{"self":{"self":{"self":{"self":null}}}}`,
		},
		{
			name:     "nil",
			input:    nil,
			expected: `null`,
		},
		{
			name:     "nil_pointer",
			input:    (*int)(nil),
			expected: `null`,
		},
		{
			name:     "nil_slice",
			input:    []int(nil),
			expected: `[]`,
		},
		{
			name:     "nil_map",
			input:    map[string]int(nil),
			expected: `{}`,
		},
		{
			name:     "nil_map_pointer",
			input:    (*map[string]int)(nil),
			expected: `null`,
		},
		{
			name:     "nil_slice_pointer",
			input:    (*[]int)(nil),
			expected: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := Marshal(tt.input)
			require.Equal(t, tt.expected, string(result))
		})
	}

	mapTests := []struct {
		name  string
		input any
	}{
		{
			name:  "map",
			input: map[string]int{"one": 1, "two": 2},
		},
		{
			name:  "map_of_struct",
			input: map[string]testStruct{"one": {Name: "John", Age: 30, Score: 8.5}, "two": {Name: "Jane", Age: 25, Score: 9.5}},
		},
		{
			name: "complex_map",
			input: map[string]any{
				"string":     "test string",
				"number":     42,
				"float":      3.14159,
				"bool":       true,
				"null_value": nil,
				"array":      []any{1, "2", 3.3, true, false, nil},
				"object": map[string]any{
					"nested": "value",
					"count":  10,
				},
			},
		},
	}

	for _, tt := range mapTests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := Marshal(tt.input)
			verify := reflect.MakeMap(reflect.TypeOf(tt.input))
			if err := stdJSON.Unmarshal(result, &verify); err != nil {
				t.Fatalf("Unmarshal(%v) error: %v", result, err)
			}
			iter := verify.MapRange()
			for iter.Next() {
				k := iter.Key()
				v := iter.Value()
				vv := reflect.ValueOf(tt.input).MapIndex(k).Interface()
				if !v.Equal(reflect.ValueOf(vv)) {
					t.Errorf("Marshal([%s]) = %v, want %v", k, v, vv)
				}
			}
		})
	}
}

func TestMarshalSyntacticEquivalence(t *testing.T) {
	testData := []any{
		"test\r\nstring",
		42,
		3.14,
		true,
		nil,
		[]int{1, 2, 3, 4, 5},
		map[string]any{
			"nested": "value",
			"count":  10,
			"bytes":  []byte("test"),
			"a":      "a\x1b[31m",
		},
		testStruct{Name: "Test", Age: 30, Score: 9.8},
	}

	for i, data := range testData {
		custom, _ := Marshal(data)
		stdlib, err := stdJSON.Marshal(data)
		if err != nil {
			t.Fatalf("Test %d: Standard Marshal error: %v", i, err)
		}

		t.Logf("custom: %s\n", custom)
		t.Logf("stdlib: %s\n", stdlib)

		// Unmarshal both into maps to compare structure equivalence
		var customMap, stdlibMap any
		if err := stdJSON.Unmarshal(custom, &customMap); err != nil {
			t.Fatalf("Test %d: Unmarshal custom error: %v", i, err)
		}
		if err := stdJSON.Unmarshal(stdlib, &stdlibMap); err != nil {
			t.Fatalf("Test %d: Unmarshal stdlib error: %v", i, err)
		}

		if !reflect.DeepEqual(customMap, stdlibMap) {
			t.Errorf("Test %d: Marshal output not equivalent.\nCustom: %s\nStdLib: %s",
				i, string(custom), string(stdlib))
		}
	}
}

func TestWithTestStruct(t *testing.T) {
	var custom, stdlib []byte
	var err error

	custom, err = Marshal(TwitterObject)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	stdlib, err = stdJSON.Marshal(TwitterObject)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var unmarshalCustom, unmarshalStdlib any
	if err := Unmarshal(custom, &unmarshalCustom); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if err := sonic.Unmarshal(stdlib, &unmarshalStdlib); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	ExpectEqual(t, unmarshalCustom, unmarshalStdlib)
}

func BenchmarkMarshalSimpleStdLib(b *testing.B) {
	testData := map[string]any{
		"string":     "test string",
		"number":     42,
		"float":      3.14159,
		"bool":       true,
		"null_value": nil,
		"bytes":      []byte("test"),
		"array":      []any{1, "2", 3.3, true, false, nil},
		"object": map[string]any{
			"nested": "value",
			"count":  10,
		},
	}

	b.Run("StdLib", func(b *testing.B) {
		for b.Loop() {
			_, _ = stdJSON.Marshal(testData)
		}
	})

	b.Run("Sonic", func(b *testing.B) {
		for b.Loop() {
			_, _ = sonic.Marshal(testData)
		}
	})

	b.Run("Custom", func(b *testing.B) {
		for b.Loop() {
			_, _ = Marshal(testData)
		}
	})
}

func BenchmarkMarshalTestStruct(b *testing.B) {
	b.Run("StdLib", func(b *testing.B) {
		for b.Loop() {
			_, _ = stdJSON.Marshal(TwitterObject)
		}
	})

	b.Run("Sonic", func(b *testing.B) {
		for b.Loop() {
			_, _ = sonic.Marshal(TwitterObject)
		}
	})

	b.Run("Custom", func(b *testing.B) {
		for b.Loop() {
			_, _ = Marshal(TwitterObject)
		}
	})
}

package serialization

import (
	"reflect"
	"testing"

	"github.com/goccy/go-yaml"
)

func BenchmarkDeserialize(b *testing.B) {
	type AnonymousStruct struct {
		J float64 `json:"j"`
		K int     `json:"k"`
	}
	type complexStruct struct {
		A string              `json:"a"`
		B int                 `json:"b"`
		C []uint              `json:"c"`
		D map[string]string   `json:"d"`
		E []map[string]string `json:"e"`
		F *complexStruct
		G struct {
			G1 float64 `json:"g1"`
			G2 int     `json:"g2"`
		}
		H []*complexStruct `json:"h"`
		*AnonymousStruct
	}
	src := SerializedObject{
		"a": "a",
		"b": "123",
		"c": "1,2,3",
		"d": "a: a\nb: b\nc: c",
		"e": "- a: a\n  b: b\n  c: c",
		"f": map[string]any{"a": "a", "b": "456", "c": `1,2,3`},
		"g": map[string]any{"g1": "1.23", "g2": 123},
		"h": []map[string]any{{"a": 123, "b": "456", "c": `["1","2","3"]`}},
		"j": "1.23",
		"k": 123,
	}
	for b.Loop() {
		dst := complexStruct{}
		err := MapUnmarshalValidate(src, &dst)
		if err != nil {
			b.Fatal(err.Error())
		}
	}
}

func BenchmarkStringToSlice(b *testing.B) {
	b.Run("ConvertString", func(b *testing.B) {
		for b.Loop() {
			dst := make([]int, 0)
			_, _ = ConvertString("- 1\n- 2\n- 3", reflect.ValueOf(&dst))
		}
	})
	b.Run("yaml.Unmarshal", func(b *testing.B) {
		for b.Loop() {
			dst := make([]int, 0)
			_ = yaml.Unmarshal([]byte("- 1\n- 2\n- 3"), &dst)
		}
	})
}

func BenchmarkStringToMap(b *testing.B) {
	b.Run("ConvertString", func(b *testing.B) {
		for b.Loop() {
			dst := make(map[string]string)
			_, _ = ConvertString("  a: b\n  c: d", reflect.ValueOf(&dst))
		}
	})
	b.Run("yaml.Unmarshal", func(b *testing.B) {
		for b.Loop() {
			dst := make(map[string]string)
			_ = yaml.Unmarshal([]byte("  a: b\n  c: d"), &dst)
		}
	})
}

func BenchmarkStringToStruct(b *testing.B) {
	dst := struct {
		A string `json:"a"`
		B int    `json:"b"`
	}{}
	b.Run("ConvertString", func(b *testing.B) {
		for b.Loop() {
			_, _ = ConvertString("  a: a\n  b: 123", reflect.ValueOf(&dst))
		}
	})
	b.Run("yaml.Unmarshal", func(b *testing.B) {
		for b.Loop() {
			_ = yaml.Unmarshal([]byte("  a: a\n  b: 123"), &dst)
		}
	})
}

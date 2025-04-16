package utils

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/goccy/go-yaml"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestUnmarshal(t *testing.T) {
	type S struct {
		I   int
		S   string
		IS  []int
		SS  []string
		MSI map[string]int
		MIS map[int]string
	}

	var (
		testStruct = S{
			I:   1,
			S:   "hello",
			IS:  []int{1, 2, 3},
			SS:  []string{"a", "b", "c"},
			MSI: map[string]int{"a": 1, "b": 2, "c": 3},
			MIS: map[int]string{1: "a", 2: "b", 3: "c"},
		}
		testStructSerialized = map[string]any{
			"I":   1,
			"S":   "hello",
			"IS":  []int{1, 2, 3},
			"SS":  []string{"a", "b", "c"},
			"MSI": map[string]int{"a": 1, "b": 2, "c": 3},
			"MIS": map[int]string{1: "a", 2: "b", 3: "c"},
		}
	)

	t.Run("unmarshal", func(t *testing.T) {
		var s2 S
		err := MapUnmarshalValidate(testStructSerialized, &s2)
		ExpectNoError(t, err)
		ExpectEqualValues(t, s2, testStruct)
	})
}

func TestUnmarshalAnonymousField(t *testing.T) {
	type Anon struct {
		A, B int
	}
	var s struct {
		Anon
		C int
	}
	var s2 struct {
		*Anon
		C int
	}
	// all, anon := extractFields(reflect.TypeOf(s2))
	// t.Fatalf("anon %v, all %v", anon, all)
	err := MapUnmarshalValidate(map[string]any{"a": 1, "b": 2, "c": 3}, &s)
	ExpectNoError(t, err)
	ExpectEqualValues(t, s.A, 1)
	ExpectEqualValues(t, s.B, 2)
	ExpectEqualValues(t, s.C, 3)

	err = MapUnmarshalValidate(map[string]any{"a": 1, "b": 2, "c": 3}, &s2)
	ExpectNoError(t, err)
	ExpectEqualValues(t, s2.A, 1)
	ExpectEqualValues(t, s2.B, 2)
	ExpectEqualValues(t, s2.C, 3)
}

func TestStringIntConvert(t *testing.T) {
	test := struct {
		I8  int8
		I16 int16
		I32 int32
		I64 int64
		U8  uint8
		U16 uint16
		U32 uint32
		U64 uint64
	}{}

	refl := reflect.ValueOf(&test)
	for i := range refl.Elem().NumField() {
		field := refl.Elem().Field(i)
		t.Run(fmt.Sprintf("field_%s", field.Type().Name()), func(t *testing.T) {
			ok, err := ConvertString("127", field)
			ExpectTrue(t, ok)
			ExpectNoError(t, err)
			ExpectEqualValues(t, field.Interface(), 127)

			err = Convert(reflect.ValueOf(uint8(64)), field)
			ExpectNoError(t, err)
			ExpectEqualValues(t, field.Interface(), 64)
		})
	}
}

type testModel struct {
	Test testType
	Baz  string
}

type testType struct {
	foo int
	bar string
}

func (c *testType) Parse(v string) (err error) {
	c.bar = v
	c.foo, err = strconv.Atoi(v)
	return
}

func TestConvertor(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		m := new(testModel)
		ExpectNoError(t, MapUnmarshalValidate(map[string]any{"Test": "123"}, m))

		ExpectEqualValues(t, m.Test.foo, 123)
		ExpectEqualValues(t, m.Test.bar, "123")
	})

	t.Run("int_to_string", func(t *testing.T) {
		m := new(testModel)
		ExpectNoError(t, MapUnmarshalValidate(map[string]any{"Test": "123"}, m))

		ExpectEqualValues(t, m.Test.foo, 123)
		ExpectEqualValues(t, m.Test.bar, "123")

		ExpectNoError(t, MapUnmarshalValidate(map[string]any{"Baz": 456}, m))
		ExpectEqualValues(t, m.Baz, "456")
	})

	t.Run("invalid", func(t *testing.T) {
		m := new(testModel)
		ExpectError(t, ErrUnsupportedConversion, MapUnmarshalValidate(map[string]any{"Test": struct{}{}}, m))
	})
}

func TestStringToSlice(t *testing.T) {
	t.Run("comma_separated", func(t *testing.T) {
		dst := make([]string, 0)
		convertible, err := ConvertString("a,b,c", reflect.ValueOf(&dst))
		ExpectTrue(t, convertible)
		ExpectNoError(t, err)
		ExpectEqualValues(t, dst, []string{"a", "b", "c"})
	})
	t.Run("yaml-like", func(t *testing.T) {
		dst := make([]string, 0)
		convertible, err := ConvertString("- a\n- b\n- c", reflect.ValueOf(&dst))
		ExpectTrue(t, convertible)
		ExpectNoError(t, err)
		ExpectEqualValues(t, dst, []string{"a", "b", "c"})
	})
	t.Run("single-line-yaml-like", func(t *testing.T) {
		dst := make([]string, 0)
		convertible, err := ConvertString("- a", reflect.ValueOf(&dst))
		ExpectTrue(t, convertible)
		ExpectNoError(t, err)
		ExpectEqualValues(t, dst, []string{"a"})
	})
}

func BenchmarkStringToSlice(b *testing.B) {
	for range b.N {
		dst := make([]int, 0)
		_, _ = ConvertString("- 1\n- 2\n- 3", reflect.ValueOf(&dst))
	}
}

func BenchmarkStringToSliceYAML(b *testing.B) {
	for range b.N {
		dst := make([]int, 0)
		_ = yaml.Unmarshal([]byte("- 1\n- 2\n- 3"), &dst)
	}
}

func TestStringToMap(t *testing.T) {
	t.Run("yaml-like", func(t *testing.T) {
		dst := make(map[string]string)
		convertible, err := ConvertString("  a: b\n  c: d", reflect.ValueOf(&dst))
		ExpectTrue(t, convertible)
		ExpectNoError(t, err)
		ExpectEqualValues(t, dst, map[string]string{"a": "b", "c": "d"})
	})
}

func BenchmarkStringToMap(b *testing.B) {
	for range b.N {
		dst := make(map[string]string)
		_, _ = ConvertString("  a: b\n  c: d", reflect.ValueOf(&dst))
	}
}

func BenchmarkStringToMapYAML(b *testing.B) {
	for range b.N {
		dst := make(map[string]string)
		_ = yaml.Unmarshal([]byte("  a: b\n  c: d"), &dst)
	}
}

func TestStringToStruct(t *testing.T) {
	type T struct {
		A string
		B int
	}
	t.Run("yaml-like simple", func(t *testing.T) {
		var dst T
		convertible, err := ConvertString("  A: a\n  B: 123", reflect.ValueOf(&dst))
		ExpectTrue(t, convertible)
		ExpectNoError(t, err)
		ExpectEqualValues(t, dst.A, "a")
		ExpectEqualValues(t, dst.B, 123)
	})

	type T2 struct {
		URL  *url.URL
		CIDR *net.IPNet
	}
	t.Run("yaml-like complex", func(t *testing.T) {
		var dst T2
		convertible, err := ConvertString("  URL: http://example.com\n  CIDR: 1.2.3.0/24", reflect.ValueOf(&dst))
		ExpectTrue(t, convertible)
		ExpectNoError(t, err)
	})
}

func BenchmarkStringToStruct(b *testing.B) {
	for range b.N {
		dst := struct {
			A string `json:"a"`
			B int    `json:"b"`
		}{}
		_, _ = ConvertString("  a: a\n  b: 123", reflect.ValueOf(&dst))
	}
}

func BenchmarkStringToStructYAML(b *testing.B) {
	for range b.N {
		dst := struct {
			A string `yaml:"a"`
			B int    `yaml:"b"`
		}{}
		_ = yaml.Unmarshal([]byte("  a: a\n  b: 123"), &dst)
	}
}

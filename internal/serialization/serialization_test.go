package serialization

import (
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestDeserialize(t *testing.T) {
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

	t.Run("deserialize", func(t *testing.T) {
		var s2 S
		err := MapUnmarshalValidate(testStructSerialized, &s2)
		ExpectNoError(t, err)
		ExpectEqual(t, s2, testStruct)
	})
}

func TestDeserializeAnonymousField(t *testing.T) {
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
	ExpectEqual(t, s.A, 1)
	ExpectEqual(t, s.B, 2)
	ExpectEqual(t, s.C, 3)

	err = MapUnmarshalValidate(map[string]any{"a": 1, "b": 2, "c": 3}, &s2)
	ExpectNoError(t, err)
	ExpectEqual(t, s2.A, 1)
	ExpectEqual(t, s2.B, 2)
	ExpectEqual(t, s2.C, 3)
}

func TestPointerPrimitives(t *testing.T) {
	type testType struct {
		B   *bool   `json:"b"`
		I8  *int8   `json:"i8"`
		I16 *int16  `json:"i16"`
		I32 *int32  `json:"i32"`
		I64 *int64  `json:"i64"`
		U8  *uint8  `json:"u8"`
		U16 *uint16 `json:"u16"`
		U32 *uint32 `json:"u32"`
		U64 *uint64 `json:"u64"`
	}
	var test testType

	err := MapUnmarshalValidate(map[string]any{"b": true, "i8": int8(127), "i16": int16(127), "i32": int32(127), "i64": int64(127), "u8": uint8(127), "u16": uint16(127), "u32": uint32(127), "u64": uint64(127)}, &test)
	ExpectNoError(t, err)
	ExpectEqual(t, *test.B, true)
	ExpectEqual(t, *test.I8, int8(127))
	ExpectEqual(t, *test.I16, int16(127))
	ExpectEqual(t, *test.I32, int32(127))
	ExpectEqual(t, *test.I64, int64(127))
	ExpectEqual(t, *test.U8, uint8(127))
	ExpectEqual(t, *test.U16, uint16(127))
	ExpectEqual(t, *test.U32, uint32(127))
	ExpectEqual(t, *test.U64, uint64(127))

	// zero values
	err = MapUnmarshalValidate(map[string]any{"b": false, "i8": int8(0), "i16": int16(0), "i32": int32(0), "i64": int64(0), "u8": uint8(0), "u16": uint16(0), "u32": uint32(0), "u64": uint64(0)}, &test)
	ExpectNoError(t, err)
	ExpectEqual(t, *test.B, false)
	ExpectEqual(t, *test.I8, int8(0))
	ExpectEqual(t, *test.I16, int16(0))
	ExpectEqual(t, *test.I32, int32(0))
	ExpectEqual(t, *test.I64, int64(0))
	ExpectEqual(t, *test.U8, uint8(0))
	ExpectEqual(t, *test.U16, uint16(0))
	ExpectEqual(t, *test.U32, uint32(0))
	ExpectEqual(t, *test.U64, uint64(0))

	// nil values
	err = MapUnmarshalValidate(map[string]any{"b": true, "i8": int8(127), "i16": int16(127), "i32": int32(127), "i64": int64(127), "u8": uint8(127), "u16": uint16(127), "u32": uint32(127), "u64": uint64(127)}, &test)
	ExpectNoError(t, err)
	err = MapUnmarshalValidate(map[string]any{"b": nil, "i8": nil, "i16": nil, "i32": nil, "i64": nil, "u8": nil, "u16": nil, "u32": nil, "u64": nil}, &test)
	ExpectNoError(t, err)
	ExpectEqual(t, test.B, nil)
	ExpectEqual(t, test.I8, nil)
	ExpectEqual(t, test.I16, nil)
	ExpectEqual(t, test.I32, nil)
	ExpectEqual(t, test.I64, nil)
	ExpectEqual(t, test.U8, nil)
	ExpectEqual(t, test.U16, nil)
	ExpectEqual(t, test.U32, nil)
	ExpectEqual(t, test.U64, nil)
}

func TestStringIntConvert(t *testing.T) {
	s := "127"

	test := struct {
		i8  int8
		i16 int16
		i32 int32
		i64 int64
		u8  uint8
		u16 uint16
		u32 uint32
		u64 uint64
	}{}

	ok, err := ConvertString(s, reflect.ValueOf(&test.i8))

	ExpectTrue(t, ok)
	ExpectNoError(t, err)
	ExpectEqual(t, test.i8, int8(127))

	ok, err = ConvertString(s, reflect.ValueOf(&test.i16))
	ExpectTrue(t, ok)
	ExpectNoError(t, err)
	ExpectEqual(t, test.i16, int16(127))

	ok, err = ConvertString(s, reflect.ValueOf(&test.i32))
	ExpectTrue(t, ok)
	ExpectNoError(t, err)
	ExpectEqual(t, test.i32, int32(127))

	ok, err = ConvertString(s, reflect.ValueOf(&test.i64))
	ExpectTrue(t, ok)
	ExpectNoError(t, err)
	ExpectEqual(t, test.i64, int64(127))

	ok, err = ConvertString(s, reflect.ValueOf(&test.u8))
	ExpectTrue(t, ok)
	ExpectNoError(t, err)
	ExpectEqual(t, test.u8, uint8(127))

	ok, err = ConvertString(s, reflect.ValueOf(&test.u16))
	ExpectTrue(t, ok)
	ExpectNoError(t, err)
	ExpectEqual(t, test.u16, uint16(127))

	ok, err = ConvertString(s, reflect.ValueOf(&test.u32))
	ExpectTrue(t, ok)
	ExpectNoError(t, err)
	ExpectEqual(t, test.u32, uint32(127))

	ok, err = ConvertString(s, reflect.ValueOf(&test.u64))
	ExpectTrue(t, ok)
	ExpectNoError(t, err)
	ExpectEqual(t, test.u64, uint64(127))
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

		ExpectEqual(t, m.Test.foo, 123)
		ExpectEqual(t, m.Test.bar, "123")
	})

	t.Run("int_to_string", func(t *testing.T) {
		m := new(testModel)
		ExpectNoError(t, MapUnmarshalValidate(map[string]any{"Test": "123"}, m))

		ExpectEqual(t, m.Test.foo, 123)
		ExpectEqual(t, m.Test.bar, "123")

		ExpectNoError(t, MapUnmarshalValidate(map[string]any{"Baz": 123}, m))
		ExpectEqual(t, m.Baz, "123")
	})

	t.Run("invalid", func(t *testing.T) {
		m := new(testModel)
		err := MapUnmarshalValidate(map[string]any{"Test": struct{ a int }{1}}, m)
		ExpectError(t, ErrUnsupportedConversion, err)
	})

	t.Run("set_empty", func(t *testing.T) {
		m := testModel{
			Test: testType{1, "2"},
			Baz:  "3",
		}
		ExpectNoError(t, MapUnmarshalValidate(map[string]any{"Test": nil, "Baz": nil}, &m))
		ExpectEqual(t, m, testModel{})
	})
}

func TestStringToSlice(t *testing.T) {
	t.Run("comma_separated", func(t *testing.T) {
		dst := make([]string, 0)
		convertible, err := ConvertString("a,b,c", reflect.ValueOf(&dst))
		ExpectTrue(t, convertible)
		ExpectNoError(t, err)
		ExpectEqual(t, dst, []string{"a", "b", "c"})
	})
	t.Run("yaml-like", func(t *testing.T) {
		dst := make([]string, 0)
		convertible, err := ConvertString("- a\n- b\n- c", reflect.ValueOf(&dst))
		ExpectTrue(t, convertible)
		ExpectNoError(t, err)
		ExpectEqual(t, dst, []string{"a", "b", "c"})
	})
	t.Run("single-line-yaml-like", func(t *testing.T) {
		dst := make([]string, 0)
		convertible, err := ConvertString("- a", reflect.ValueOf(&dst))
		ExpectTrue(t, convertible)
		ExpectNoError(t, err)
		ExpectEqual(t, dst, []string{"a"})
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
		ExpectEqual(t, dst, map[string]string{"a": "b", "c": "d"})
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
	t.Run("yaml-like", func(t *testing.T) {
		dst := struct {
			A string
			B int
		}{}
		convertible, err := ConvertString("  A: a\n  B: 123", reflect.ValueOf(&dst))
		ExpectTrue(t, convertible)
		ExpectNoError(t, err)
		ExpectEqual(t, dst, struct {
			A string
			B int
		}{"a", 123})
	})
}

func TestConfigEnvSubstitution(t *testing.T) {
	os.Setenv("CLOUDFLARE_AUTH_TOKEN", "test")
	data := []byte(`
---
autocert:
  options:
    auth_token: ${CLOUDFLARE_AUTH_TOKEN}
`)

	var cfg struct {
		Autocert struct {
			Options struct {
				AuthToken string `yaml:"auth_token"`
			} `yaml:"options"`
		} `yaml:"autocert"`
	}
	require.NoError(t, UnmarshalValidateYAML(data, &cfg))
	require.Equal(t, "test", cfg.Autocert.Options.AuthToken)
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

package json

import "reflect"

type checkEmptyFunc func(v reflect.Value) bool

var checkEmptyFuncs = map[reflect.Kind]checkEmptyFunc{
	reflect.String:        checkStringEmpty,
	reflect.Int:           checkIntEmpty,
	reflect.Int8:          checkIntEmpty,
	reflect.Int16:         checkIntEmpty,
	reflect.Int32:         checkIntEmpty,
	reflect.Int64:         checkIntEmpty,
	reflect.Uint:          checkUintEmpty,
	reflect.Uint8:         checkUintEmpty,
	reflect.Uint16:        checkUintEmpty,
	reflect.Uint32:        checkUintEmpty,
	reflect.Uint64:        checkUintEmpty,
	reflect.Float32:       checkFloatEmpty,
	reflect.Float64:       checkFloatEmpty,
	reflect.Bool:          checkBoolEmpty,
	reflect.Slice:         checkLenEmpty,
	reflect.Map:           checkLenEmpty,
	reflect.Array:         checkLenEmpty,
	reflect.Chan:          reflect.Value.IsNil,
	reflect.Func:          reflect.Value.IsNil,
	reflect.Interface:     reflect.Value.IsNil,
	reflect.Pointer:       reflect.Value.IsNil,
	reflect.Struct:        reflect.Value.IsZero,
	reflect.UnsafePointer: reflect.Value.IsNil,
}

func checkStringEmpty(v reflect.Value) bool {
	return v.String() == ""
}

func checkIntEmpty(v reflect.Value) bool {
	return v.Int() == 0
}

func checkUintEmpty(v reflect.Value) bool {
	return v.Uint() == 0
}

func checkFloatEmpty(v reflect.Value) bool {
	return v.Float() == 0
}

func checkBoolEmpty(v reflect.Value) bool {
	return !v.Bool()
}

func checkLenEmpty(v reflect.Value) bool {
	return v.Len() == 0
}

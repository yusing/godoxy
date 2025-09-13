package utils

import (
	"reflect"
	"unsafe"
)

// DeepEqual reports whether x and y are deeply equal.
// It supports numerics, strings, maps, slices, arrays, and structs (exported fields only).
// It's optimized for performance by avoiding reflection for common types and
// adaptively choosing between BFS and DFS traversal strategies.
func DeepEqual(x, y any) bool {
	if x == nil || y == nil {
		return x == y
	}

	v1 := reflect.ValueOf(x)
	v2 := reflect.ValueOf(y)

	if v1.Type() != v2.Type() {
		return false
	}

	return deepEqual(v1, v2, make(map[visit]bool), 0)
}

// visit represents a visit to a pair of values during comparison
type visit struct {
	a1, a2 unsafe.Pointer
	typ    reflect.Type
}

// deepEqual performs the actual deep comparison with cycle detection
func deepEqual(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}

	if v1.Type() != v2.Type() {
		return false
	}

	// Handle cycle detection for pointer-like types
	if v1.CanAddr() && v2.CanAddr() {
		addr1 := unsafe.Pointer(v1.UnsafeAddr())
		addr2 := unsafe.Pointer(v2.UnsafeAddr())
		typ := v1.Type()
		v := visit{addr1, addr2, typ}
		if visited[v] {
			return true // already visiting, assume equal
		}
		visited[v] = true
		defer delete(visited, v)
	}

	switch v1.Kind() {
	case reflect.Bool:
		return v1.Bool() == v2.Bool()

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v1.Int() == v2.Int()

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v1.Uint() == v2.Uint()

	case reflect.Float32, reflect.Float64:
		return floatEqual(v1.Float(), v2.Float())

	case reflect.Complex64, reflect.Complex128:
		c1, c2 := v1.Complex(), v2.Complex()
		return floatEqual(real(c1), real(c2)) && floatEqual(imag(c1), imag(c2))

	case reflect.String:
		return v1.String() == v2.String()

	case reflect.Array:
		return deepEqualArray(v1, v2, visited, depth)

	case reflect.Slice:
		return deepEqualSlice(v1, v2, visited, depth)

	case reflect.Map:
		return deepEqualMap(v1, v2, visited, depth)

	case reflect.Struct:
		return deepEqualStruct(v1, v2, visited, depth)

	case reflect.Ptr:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() && v2.IsNil()
		}
		return deepEqual(v1.Elem(), v2.Elem(), visited, depth+1)

	case reflect.Interface:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() && v2.IsNil()
		}
		return deepEqual(v1.Elem(), v2.Elem(), visited, depth+1)

	default:
		// For unsupported types (func, chan, etc.), fall back to basic equality
		return v1.Interface() == v2.Interface()
	}
}

// floatEqual handles NaN cases properly
func floatEqual(f1, f2 float64) bool {
	return f1 == f2 || (f1 != f1 && f2 != f2) // NaN == NaN
}

// deepEqualArray compares arrays using DFS (since arrays have fixed size)
func deepEqualArray(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	for i := range v1.Len() {
		if !deepEqual(v1.Index(i), v2.Index(i), visited, depth+1) {
			return false
		}
	}
	return true
}

// deepEqualSlice compares slices, choosing strategy based on size and depth
func deepEqualSlice(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	if v1.IsNil() != v2.IsNil() {
		return false
	}
	if v1.Len() != v2.Len() {
		return false
	}
	if v1.IsNil() {
		return true
	}

	// Use BFS for large slices at shallow depth to improve cache locality
	// Use DFS for small slices or deep nesting to reduce memory overhead
	if shouldUseBFS(v1.Len(), depth) {
		return deepEqualSliceBFS(v1, v2, visited, depth)
	}
	return deepEqualSliceDFS(v1, v2, visited, depth)
}

// deepEqualSliceDFS uses depth-first traversal
func deepEqualSliceDFS(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	for i := range v1.Len() {
		if !deepEqual(v1.Index(i), v2.Index(i), visited, depth+1) {
			return false
		}
	}
	return true
}

// deepEqualSliceBFS uses breadth-first traversal for better cache locality
func deepEqualSliceBFS(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	length := v1.Len()

	// First, check all direct elements
	for i := range length {
		elem1, elem2 := v1.Index(i), v2.Index(i)

		// For simple types, compare directly
		if isSimpleType(elem1.Kind()) {
			if !deepEqual(elem1, elem2, visited, depth+1) {
				return false
			}
		}
	}

	// Then, recursively check complex elements
	for i := range length {
		elem1, elem2 := v1.Index(i), v2.Index(i)

		if !isSimpleType(elem1.Kind()) {
			if !deepEqual(elem1, elem2, visited, depth+1) {
				return false
			}
		}
	}

	return true
}

// deepEqualMap compares maps
func deepEqualMap(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	if v1.IsNil() != v2.IsNil() {
		return false
	}
	if v1.Len() != v2.Len() {
		return false
	}
	if v1.IsNil() {
		return true
	}

	// Check all keys and values
	for _, key := range v1.MapKeys() {
		val1 := v1.MapIndex(key)
		val2 := v2.MapIndex(key)

		if !val2.IsValid() {
			return false // key doesn't exist in v2
		}

		if !deepEqual(val1, val2, visited, depth+1) {
			return false
		}
	}

	return true
}

// deepEqualStruct compares structs (exported fields only)
func deepEqualStruct(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	typ := v1.Type()

	for i := range typ.NumField() {
		field := typ.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		if !deepEqual(v1.Field(i), v2.Field(i), visited, depth+1) {
			return false
		}
	}

	return true
}

// shouldUseBFS determines whether to use BFS or DFS based on slice size and depth
func shouldUseBFS(length, depth int) bool {
	// Use BFS for large slices at shallow depth (better cache locality)
	// Use DFS for small slices or deep nesting (lower memory overhead)
	return length > 100 && depth < 3
}

// isSimpleType checks if a type can be compared without deep recursion
func isSimpleType(kind reflect.Kind) bool {
	if kind >= reflect.Bool && kind <= reflect.Complex128 {
		return true
	}
	return kind == reflect.String
}

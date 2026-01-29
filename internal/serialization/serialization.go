package serialization

import (
	"errors"
	"io"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/bytedance/sonic"
	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
	"github.com/puzpuzpuz/xsync/v4"
	gi "github.com/yusing/gointernals"
	"github.com/yusing/goutils/env"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

type SerializedObject = map[string]any

// ToSerializedObject converts a map[string]VT to a SerializedObject.
func ToSerializedObject[VT any](m map[string]VT) SerializedObject {
	so := make(SerializedObject, len(m))
	for k, v := range m {
		so[k] = v
	}
	return so
}

func init() {
	strutils.SetJSONMarshaler(sonic.Marshal)
	strutils.SetJSONUnmarshaler(sonic.Unmarshal)
	strutils.SetYAMLMarshaler(yaml.Marshal)
	strutils.SetYAMLUnmarshaler(yaml.Unmarshal)
}

type MapUnmarshaller interface {
	UnmarshalMap(m map[string]any) gperr.Error
}

var (
	ErrInvalidType           = gperr.New("invalid type")
	ErrNilValue              = gperr.New("nil")
	ErrUnsettable            = gperr.New("unsettable")
	ErrUnsupportedConversion = gperr.New("unsupported conversion")
	ErrUnknownField          = gperr.New("unknown field")
)

var (
	tagDeserialize = "deserialize" // `deserialize:"-"` to exclude from deserialization
	tagJSON        = "json"        // share between Deserialize and json.Marshal
	tagValidate    = "validate"    // uses go-playground/validator
	tagAliases     = "aliases"     // declare aliases for fields
)

var mapUnmarshalerType = reflect.TypeFor[MapUnmarshaller]()

var defaultValues = make(map[reflect.Type]func() any)

// RegisterDefaultValueFactory registers a factory function for a type.
// This is not concurrent safe. Intended to be used in init functions.
func RegisterDefaultValueFactory[T any](factory func() *T) {
	t := reflect.TypeFor[T]()
	if t.Kind() == reflect.Pointer {
		panic("pointer of pointer")
	}
	if _, ok := defaultValues[t]; ok {
		panic("default value for " + t.String() + " already registered")
	}
	defaultValues[t] = func() any { return factory() }
}

// initPtr initialize the ptr with default value if exists,
// otherwise, initialize the ptr with zero value.
func initPtr(dst reflect.Value) {
	dstT := dst.Type()
	if dv, ok := defaultValues[dstT.Elem()]; ok {
		dst.Set(reflect.ValueOf(dv()))
	} else {
		gi.ReflectInitPtr(dst)
	}
}

// Validate performs struct validation using go-playground/validator tags.
//
// It collects all validation errors and returns them as a single error.
// Field names in errors are prefixed with their namespace (e.g., "User.Email").
func ValidateWithFieldTags(s any) gperr.Error {
	var errs gperr.Builder
	err := validate.Struct(s)
	var valErrs validator.ValidationErrors
	if errors.As(err, &valErrs) {
		for _, e := range valErrs {
			detail := e.ActualTag()
			if e.Param() != "" {
				detail += ":" + e.Param()
			}
			if detail != "required" {
				detail = "require " + strconv.Quote(detail)
			}
			errs.Add(ErrValidationError.
				Subject(e.Namespace()).
				Withf(detail))
		}
	}
	return errs.Error()
}

func dive(dst reflect.Value) (v reflect.Value, t reflect.Type, err gperr.Error) {
	dstT := dst.Type()
	for {
		switch dstT.Kind() {
		case reflect.Pointer:
			dst = dst.Elem()
			dstT = dstT.Elem()
		default:
			return dst, dstT, nil
		}
	}
}

func fnv1IgnoreCaseSnake(s string) uint32 {
	const (
		offset32 uint32 = 2166136261
		prime32  uint32 = 16777619
	)
	hash := offset32
	// range over bytes instead of runes
	for i := range s {
		r := s[i]
		if r == '_' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		hash = hash*prime32 ^ uint32(r)
	}
	return hash
}

type typeInfo struct {
	keyFieldIndexes map[uint32][]int
	fieldNames      map[string]struct{}
	hasValidateTag  bool
}

func (t typeInfo) getField(v reflect.Value, k string) (reflect.Value, bool) {
	hash := fnv1IgnoreCaseSnake(k)
	if field, ok := t.keyFieldIndexes[hash]; ok {
		return fieldByIndexWithLazyPtrInitialization(v, field), true
	}
	return reflect.Value{}, false
}

func fieldByIndexWithLazyPtrInitialization(v reflect.Value, index []int) reflect.Value {
	if len(index) == 1 {
		return v.Field(index[0])
	}
	for i, x := range index {
		if i > 0 {
			if v.Kind() == reflect.Pointer && v.Type().Elem().Kind() == reflect.Struct {
				if v.IsNil() {
					initPtr(v)
				}
				v = v.Elem()
			}
		}
		v = v.Field(x)
	}
	return v
}

var getTypeInfo func(t reflect.Type) typeInfo

func init() {
	m := xsync.NewMap[reflect.Type, typeInfo](xsync.WithGrowOnly(), xsync.WithPresize(100))
	getTypeInfo = func(t reflect.Type) typeInfo {
		ti, _ := m.LoadOrCompute(t, func() (typeInfo, bool) {
			return initTypeKeyFieldIndexesMap(t), false
		})
		return ti
	}
}

func initTypeKeyFieldIndexesMap(t reflect.Type) typeInfo {
	hasValidateTag := false
	numFields := t.NumField()

	keyFieldIndexes := make(map[uint32][]int, numFields)
	fieldNames := make(map[string]struct{}, numFields)

	for i := range numFields {
		field := t.Field(i)
		deserializeTag := field.Tag.Get(tagDeserialize)
		jsonTag := field.Tag.Get(tagJSON)

		if jsonTag != "" {
			jsonTag, _, _ = strings.Cut(jsonTag, ",")
		}

		if deserializeTag == "-" || jsonTag == "-" {
			continue
		}

		if !field.IsExported() {
			continue
		}
		if field.Anonymous {
			fieldT := field.Type
			if fieldT.Kind() == reflect.Pointer {
				fieldT = fieldT.Elem()
			}
			if fieldT.Kind() != reflect.Struct {
				goto notAnonymousStruct
			}
			typeInfo := getTypeInfo(fieldT)
			for k, v := range typeInfo.keyFieldIndexes {
				keyFieldIndexes[k] = append(field.Index, v...)
			}
			for k := range typeInfo.fieldNames {
				fieldNames[k] = struct{}{}
			}
			hasValidateTag = hasValidateTag || typeInfo.hasValidateTag
			continue
		}
	notAnonymousStruct:
		var key string
		if jsonTag != "" {
			key = jsonTag
			if idxComma := strings.Index(key, ","); idxComma != -1 {
				key = key[:idxComma]
			}
		} else {
			key = field.Name
		}
		keyFieldIndexes[fnv1IgnoreCaseSnake(key)] = field.Index
		fieldNames[key] = struct{}{}

		if !hasValidateTag {
			_, hasValidateTag = field.Tag.Lookup(tagValidate)
		}

		aliases := field.Tag.Get(tagAliases)
		if aliases != "" {
			for alias := range strings.SplitSeq(aliases, ",") {
				keyFieldIndexes[fnv1IgnoreCaseSnake(alias)] = field.Index
				fieldNames[alias] = struct{}{}
			}
		}
	}

	return typeInfo{
		keyFieldIndexes: keyFieldIndexes,
		fieldNames:      fieldNames,
		hasValidateTag:  hasValidateTag,
	}
}

// MapUnmarshalValidate takes a SerializedObject and a target value,
// and assigns the values in the SerializedObject to the target value.
//
// It ignores case differences between the field names in the SerializedObject and the target.
//
// If the target value is a struct , and implements the MapUnmarshaller interface,
// the UnmarshalMap method will be called.
//
// If the target value is a struct, but does not implements the MapUnmarshaller interface,
// the SerializedObject will be deserialized into the struct fields and validate if needed.
//
// If the target value is a map[string]any the SerializedObject will be deserialized into the map.
//
// The function returns an error if the target value is not a struct or a map[string]any, or if there is an error during deserialization.
func MapUnmarshalValidate(src SerializedObject, dst any) (err gperr.Error) {
	return mapUnmarshalValidate(src, reflect.ValueOf(dst), true)
}

func mapUnmarshalValidate(src SerializedObject, dstV reflect.Value, checkValidateTag bool) (err gperr.Error) {
	dstT := dstV.Type()

	if src != nil && dstT.Implements(mapUnmarshalerType) {
		dstV, _, err = dive(dstV)
		if err != nil {
			return err
		}
		return dstV.Addr().Interface().(MapUnmarshaller).UnmarshalMap(src)
	}

	dstV, dstT, err = dive(dstV)
	if err != nil {
		return err
	}

	if src == nil {
		if dstV.CanSet() {
			dstV.SetZero()
			return nil
		}
		return gperr.Errorf("deserialize: src is %w and dst is not settable", ErrNilValue)
	}

	// convert data fields to lower no-snake
	// convert target fields to lower no-snake
	// then check if the field of data is in the target

	var errs gperr.Builder

	switch dstV.Kind() {
	case reflect.Struct, reflect.Interface:
		info := getTypeInfo(dstT)
		for k, v := range src {
			if field, ok := info.getField(dstV, k); ok {
				err := Convert(reflect.ValueOf(v), field, !info.hasValidateTag)
				if err != nil {
					errs.Add(err.Subject(k))
				}
			} else {
				errs.Add(ErrUnknownField.Subject(k).With(gperr.DoYouMeanField(k, info.fieldNames)))
			}
		}
		if info.hasValidateTag && checkValidateTag {
			errs.Add(ValidateWithFieldTags(dstV.Interface()))
		}
		if err := ValidateWithCustomValidator(dstV); err != nil {
			errs.Add(err)
		}
		return errs.Error()
	case reflect.Map:
		if dstV.IsNil() {
			if !dstV.CanSet() {
				return gperr.Errorf("dive: dst is %w and is not settable", ErrNilValue)
			}
			gi.ReflectInitMap(dstV, len(src))
		}
		if dstT.Key().Kind() != reflect.String {
			return gperr.Errorf("deserialize: %w for map of non string keys (map of %s)", ErrUnsupportedConversion, dstT.Elem().String())
		}
		// ?: should we clear the  map?
		for k, v := range src {
			elem := gi.ReflectStrMapAssign(dstV, k)
			err := Convert(reflect.ValueOf(v), elem, true)
			if err != nil {
				errs.Add(err.Subject(k))
				continue
			}
			if err := ValidateWithCustomValidator(elem); err != nil {
				errs.Add(err.Subject(k))
			}
		}
		if err := ValidateWithCustomValidator(dstV); err != nil {
			errs.Add(err)
		}
		return errs.Error()
	default:
		return ErrUnsupportedConversion.Subject("mapping to " + dstT.String() + " ")
	}
}

// Convert attempts to convert the src to dst.
//
// If src is a map, it is deserialized into dst.
// If src is a slice, each of its elements are converted and stored in dst.
// For any other type, it is converted using the reflect.Value.Convert function (if possible).
//
// If dst is not settable, an error is returned.
// If src cannot be converted to dst, an error is returned.
// If any error occurs during conversion (e.g. deserialization), it is returned.
//
// Returns:
//   - error: the error occurred during conversion, or nil if no error occurred.
func Convert(src reflect.Value, dst reflect.Value, checkValidateTag bool) gperr.Error {
	if !dst.IsValid() {
		return gperr.Errorf("convert: dst is %w", ErrNilValue)
	}

	if (src.Kind() == reflect.Pointer && src.IsNil()) || !src.IsValid() {
		if !dst.CanSet() {
			return gperr.Errorf("convert: src is %w", ErrNilValue)
		}
		dst.SetZero()
		return nil
	}

	if src.IsZero() {
		if !dst.CanSet() {
			return gperr.Errorf("convert: src is %w", ErrNilValue)
		}
		switch dst.Kind() {
		case reflect.Pointer:
			initPtr(dst)
		default:
			dst.SetZero()
		}
		return nil
	}

	srcT := src.Type()
	dstT := dst.Type()

	if src.Kind() == reflect.Interface {
		src = src.Elem()
		srcT = src.Type()
	}

	if dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			if !dst.CanSet() {
				return ErrUnsettable.Subject(dstT.String())
			}
			initPtr(dst)
		}
		dst = dst.Elem()
		dstT = dst.Type()
	}

	srcKind := srcT.Kind()

	switch {
	case srcT == dstT, srcT.AssignableTo(dstT):
		if !dst.CanSet() {
			return ErrUnsettable.Subject(dstT.String())
		}
		dst.Set(src)
		return nil
	case srcKind == reflect.String:
		if !dst.CanSet() {
			return ErrUnsettable.Subject(dstT.String())
		}
		if convertible, err := ConvertString(src.String(), dst); convertible {
			return err
		}
	case gi.ReflectIsNumeric(src) && gi.ReflectIsNumeric(dst):
		dst.Set(src.Convert(dst.Type()))
		return nil
	case gi.ReflectIsNumeric(src):
		// try ConvertString
		if convertible, err := ConvertString(gi.ReflectToStr(src), dst); convertible {
			return err
		}
	case dstT.Kind() == reflect.String:
		dst.SetString(gi.ReflectToStr(src))
		return nil
	case srcKind == reflect.Map: // map to map
		if src.Len() == 0 {
			return nil
		}
		obj, ok := src.Interface().(SerializedObject)
		if !ok {
			return ErrUnsupportedConversion.Subject(dstT.String() + " to " + srcT.String())
		}
		return mapUnmarshalValidate(obj, dst.Addr(), checkValidateTag)
	case srcKind == reflect.Slice: // slice to slice
		return ConvertSlice(src, dst, checkValidateTag)
	}

	return ErrUnsupportedConversion.Subjectf("%s to %s", srcT, dstT)
}

// ConvertSlice converts a source slice to a destination slice.
//
//   - Elements are converted one by one using the Convert function.
//   - Validation is performed on each element if checkValidateTag is true.
//   - The destination slice is initialized with the source length.
//   - On error, the destination slice is truncated to the number of
//     successfully converted elements.
func ConvertSlice(src reflect.Value, dst reflect.Value, checkValidateTag bool) gperr.Error {
	if dst.Kind() == reflect.Pointer {
		if dst.IsNil() && !dst.CanSet() {
			return ErrNilValue
		}
		initPtr(dst)
		dst = dst.Elem()
	}

	if !dst.CanSet() {
		return ErrUnsettable.Subject(dst.Type().String())
	}

	if src.Kind() != reflect.Slice {
		return Convert(src, dst, checkValidateTag)
	}

	srcLen := src.Len()
	if srcLen == 0 {
		dst.SetZero()
		return nil
	}
	if dst.Kind() != reflect.Slice {
		return ErrUnsupportedConversion.Subjectf("%s to %s", dst.Type(), src.Type())
	}

	var sliceErrs gperr.Builder
	numValid := 0
	gi.ReflectInitSlice(dst, srcLen, srcLen)
	for j := range srcLen {
		err := Convert(src.Index(j), dst.Index(numValid), checkValidateTag)
		if err != nil {
			sliceErrs.Add(err.Subjectf("[%d]", j))
			continue
		}
		numValid++
	}

	if dst.Type().Implements(reflect.TypeFor[CustomValidator]()) {
		err := dst.Interface().(CustomValidator).Validate()
		if err != nil {
			sliceErrs.Add(err)
		}
	}

	if err := sliceErrs.Error(); err != nil {
		dst.SetLen(numValid) // shrink to number of elements that were successfully converted
		return err
	}
	return nil
}

// ConvertString converts a string value to the destination reflect.Value.
//   - It handles various types including numeric types, booleans, time.Duration,
//     slices (comma-separated or YAML), maps, and structs (YAML).
//   - If the destination implements the Parser interface, it is used for conversion.
//   - Returns true if conversion was handled (even with error), false if
//     conversion is unsupported.
func ConvertString(src string, dst reflect.Value) (convertible bool, convErr gperr.Error) {
	convertible = true
	dstT := dst.Type()
	if dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			// Early return for empty string
			if src == "" {
				return true, nil
			}
			initPtr(dst)
		}
		dst = dst.Elem()
		dstT = dst.Type()
	}

	// Early return for empty string
	if src == "" {
		dst.SetZero()
		return true, nil
	}

	if dst.Kind() == reflect.String {
		dst.SetString(src)
		return true, nil
	}

	// check if (*T).Convertor is implemented
	if addr := dst.Addr(); addr.Type().Implements(reflect.TypeFor[strutils.Parser]()) {
		parser := addr.Interface().(strutils.Parser)
		return true, gperr.Wrap(parser.Parse(src))
	}

	switch dstT {
	case reflect.TypeFor[time.Duration]():
		d, err := time.ParseDuration(src)
		if err != nil {
			return true, gperr.Wrap(err)
		}
		gi.ReflectValueSet(dst, d)
		return true, nil
	default:
	}

	if gi.ReflectIsNumeric(dst) || dst.Kind() == reflect.Bool {
		err := gi.ReflectStrToNumBool(dst, src)
		if err != nil {
			return true, gperr.Wrap(err)
		}
		return true, nil
	}

	// yaml like
	switch dst.Kind() {
	case reflect.Slice:
		// one liner is comma separated list
		isMultiline := strings.IndexByte(src, '\n') != -1
		if !isMultiline && src[0] != '-' && src[0] != '[' {
			values := strutils.CommaSeperatedList(src)
			size := len(values)
			gi.ReflectInitSlice(dst, size, size)
			var errs gperr.Builder
			for i, v := range values {
				_, err := ConvertString(v, dst.Index(i))
				if err != nil {
					errs.AddSubjectf(err, "[%d]", i)
				}
			}
			if errs.HasError() {
				return true, errs.Error()
			}
			return true, nil
		}

		sl := []any{}
		err := yaml.Unmarshal(unsafe.Slice(unsafe.StringData(src), len(src)), &sl)
		if err != nil {
			return true, gperr.Wrap(err)
		}
		return true, ConvertSlice(reflect.ValueOf(sl), dst, true)
	case reflect.Map, reflect.Struct:
		rawMap := SerializedObject{}
		err := yaml.Unmarshal(unsafe.Slice(unsafe.StringData(src), len(src)), &rawMap)
		if err != nil {
			return true, gperr.Wrap(err)
		}
		return true, mapUnmarshalValidate(rawMap, dst, true)
	default:
		return false, nil
	}
}

var envRegex = regexp.MustCompile(`\$\{([^}]+)\}`) // e.g. ${CLOUDFLARE_API_KEY}

func substituteEnv(data []byte) ([]byte, gperr.Error) {
	envError := gperr.NewBuilder("env substitution error")
	data = envRegex.ReplaceAllFunc(data, func(match []byte) []byte {
		varName := string(match[2 : len(match)-1])
		// NOTE: use env.LookupEnv instead of os.LookupEnv to support environment variable prefixes
		// like ${API_ADDR} will lookup for GODOXY_API_ADDR, GOPROXY_API_ADDR and API_ADDR.
		env, ok := env.LookupEnv(varName)
		if !ok {
			envError.Addf("%s is not set", varName)
		}
		return strconv.AppendQuote(nil, env)
	})
	if envError.HasError() {
		return nil, envError.Error()
	}
	return data, nil
}

type (
	marshalFunc    func(src any) ([]byte, error)
	unmarshalFunc  func(data []byte, target any) error
	newDecoderFunc func(r io.Reader) interface {
		Decode(v any) error
	}
	interceptFunc func(m map[string]any) gperr.Error
)

// UnmarshalValidate unmarshals data into a map, applies optional intercept
// functions, and validates the result against the target struct using field tags.
//   - Environment variables in the data are substituted using ${VAR} syntax.
//   - The unmarshaler function converts data to a map[string]any.
//   - Intercept functions can modify or validate the map before unmarshaling.
func UnmarshalValidate[T any](data []byte, target *T, unmarshaler unmarshalFunc, interceptFns ...interceptFunc) gperr.Error {
	data, err := substituteEnv(data)
	if err != nil {
		return err
	}

	m := make(map[string]any)
	if err := unmarshaler(data, &m); err != nil {
		return gperr.Wrap(err)
	}
	for _, intercept := range interceptFns {
		if err := intercept(m); err != nil {
			return err
		}
	}
	return MapUnmarshalValidate(m, target)
}

// UnmarshalValidateReader reads from an io.Reader, unmarshals to a map,
//   - Applies optional intercept functions, and validates against the target struct.
//   - Environment variables are substituted during reading using ${VAR} syntax.
//   - The newDecoder function creates a decoder for the reader (e.g.,
//     json.NewDecoder).
func UnmarshalValidateReader[T any](reader io.Reader, target *T, newDecoder newDecoderFunc, interceptFns ...interceptFunc) gperr.Error {
	m := make(map[string]any)
	if err := newDecoder(NewSubstituteEnvReader(reader)).Decode(&m); err != nil {
		return gperr.Wrap(err)
	}
	for _, intercept := range interceptFns {
		if err := intercept(m); err != nil {
			return err
		}
	}
	return MapUnmarshalValidate(m, target)
}

// UnmarshalValidateXSync unmarshals data into an xsync.Map[string, V].
//   - Environment variables in the data are substituted using ${VAR} syntax.
//   - The unmarshaler function converts data to a map[string]any.
//   - Intercept functions can modify or validate the map before unmarshaling.
//   - Returns a thread-safe concurrent map with the unmarshaled values.
func UnmarshalValidateXSync[V any](data []byte, unmarshaler unmarshalFunc, interceptFns ...interceptFunc) (*xsync.Map[string, V], gperr.Error) {
	data, err := substituteEnv(data)
	if err != nil {
		return nil, err
	}

	m := make(map[string]any)
	if err := unmarshaler(data, &m); err != nil {
		return nil, gperr.Wrap(err)
	}
	for _, intercept := range interceptFns {
		if err := intercept(m); err != nil {
			return nil, err
		}
	}

	m2 := make(map[string]V, len(m))
	if err = MapUnmarshalValidate(m, m2); err != nil {
		return nil, err
	}
	ret := xsync.NewMap[string, V](xsync.WithPresize(len(m)))
	for k, v := range m2 {
		ret.Store(k, v)
	}
	return ret, nil
}

// SaveFile marshals a value to bytes and writes it to a file.
//   - The marshaler function converts the value to bytes.
//   - The file is written with the specified permissions.
func SaveFile[T any](path string, src *T, perm os.FileMode, marshaler marshalFunc) error {
	data, err := marshaler(src)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// LoadFileIfExist reads a file and unmarshals its contents to a value.
//   - The unmarshaler function converts the bytes to a value.
//   - If the file does not exist, nil is returned and dst remains unchanged.
func LoadFileIfExist[T any](path string, dst *T, unmarshaler unmarshalFunc) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return unmarshaler(data, dst)
}

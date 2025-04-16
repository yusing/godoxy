package utils

import (
	"bytes"
	"errors"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/yusing/go-proxy/pkg/json"

	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils/functional"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type SerializedObject = map[string]any

type (
	MapMarshaler interface {
		MarshalMap() map[string]any
	}
	MapUnmarshaler interface {
		UnmarshalMap(m map[string]any) gperr.Error
	}
	Marshaler interface {
		MarshalJSONTo(buf *bytes.Buffer) error
	}
)

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

var (
	typeDuration = reflect.TypeFor[time.Duration]()
	typeTime     = reflect.TypeFor[time.Time]()
	typeURL      = reflect.TypeFor[url.URL]()
	typeCIDR     = reflect.TypeFor[net.IPNet]()

	typeMapUnmarshaler = reflect.TypeFor[MapUnmarshaler]()
	typeStrParser      = reflect.TypeFor[strutils.Parser]()
)

var defaultValues = functional.NewMapOf[reflect.Type, func() any]()

func RegisterDefaultValueFactory[T any](factory func() *T) {
	t := reflect.TypeFor[T]()
	if t.Kind() == reflect.Ptr {
		panic("pointer of pointer")
	}
	if defaultValues.Has(t) {
		panic("default value for " + t.String() + " already registered")
	}
	defaultValues.Store(t, func() any { return factory() })
}

func New(t reflect.Type) reflect.Value {
	if dv, ok := defaultValues.Load(t); ok {
		return reflect.ValueOf(dv())
	}
	return reflect.New(t)
}

func extractFields(t reflect.Type) (all, anonymous []reflect.StructField) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, nil
	}
	n := t.NumField()
	fields := make([]reflect.StructField, 0, n)
	for i := range n {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		// not checking tagJSON because json:"-" is for skipping json.Marshal
		if field.Tag.Get(tagDeserialize) == "-" {
			continue
		}
		if field.Anonymous {
			nested, _ := extractFields(field.Type)
			fields = append(fields, nested...)
			anonymous = append(anonymous, field)
		} else {
			fields = append(fields, field)
		}
	}
	return fields, anonymous
}

func ValidateWithFieldTags(s any) gperr.Error {
	errs := gperr.NewBuilder("validate error")
	err := validate.Struct(s)
	var valErrs validator.ValidationErrors
	if errors.As(err, &valErrs) {
		for _, e := range valErrs {
			detail := e.ActualTag()
			if e.Param() != "" {
				detail += ":" + e.Param()
			}
			errs.Add(ErrValidationError.
				Subject(e.Namespace()).
				Withf("require %q", detail))
		}
	}
	return errs.Error()
}

func ValidateWithCustomValidator(v reflect.Value) gperr.Error {
	isStruct := false
	for {
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if v.IsNil() {
				return gperr.Errorf("validate: v is %w", ErrNilValue)
			}
			if validate, ok := v.Interface().(CustomValidator); ok {
				return validate.Validate()
			}
			if isStruct {
				return nil
			}
			v = v.Elem()
		case reflect.Struct:
			if !v.CanAddr() {
				return nil
			}
			v = v.Addr()
			isStruct = true
		default:
			return nil
		}
	}
}

func dive(dst reflect.Value) (v reflect.Value, t reflect.Type, err gperr.Error) {
	dstT := dst.Type()
	for {
		switch dst.Kind() {
		case reflect.Pointer, reflect.Interface:
			if dst.IsNil() {
				if !dst.CanSet() {
					err = gperr.Errorf("dive: dst is %w and is not settable", ErrNilValue)
					return
				}
				dst.Set(New(dstT.Elem()))
			}
			dst = dst.Elem()
			dstT = dst.Type()
		case reflect.Struct:
			return dst, dstT, nil
		default:
			if dst.IsNil() {
				switch dst.Kind() {
				case reflect.Map:
					dst.Set(reflect.MakeMap(dstT))
				case reflect.Slice:
					dst.Set(reflect.MakeSlice(dstT, 0, 0))
				default:
					err = gperr.Errorf("dive: %w for dst %s", ErrInvalidType, dstT.String())
					return
				}
			}
			return dst, dstT, nil
		}
	}
}

// MapUnmarshalValidate takes a SerializedObject and a target value, and assigns the values in the SerializedObject to the target value.
// MapUnmarshalValidate ignores case differences between the field names in the SerializedObject and the target.
//
// The target value must be a struct or a map[string]any.
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
	dstV := reflect.ValueOf(dst)
	dstT := dstV.Type()

	if src == nil {
		if dstV.CanSet() {
			dstV.Set(reflect.Zero(dstT))
			return nil
		}
		return gperr.Errorf("unmarshal: src is %w and dst is not settable", ErrNilValue)
	}

	if dstT.Implements(typeMapUnmarshaler) {
		dstV, _, err = dive(dstV)
		if err != nil {
			return err
		}
		return dstV.Addr().Interface().(MapUnmarshaler).UnmarshalMap(src)
	}

	dstV, dstT, err = dive(dstV)
	if err != nil {
		return err
	}

	// convert data fields to lower no-snake
	// convert target fields to lower no-snake
	// then check if the field of data is in the target

	errs := gperr.NewBuilder()

	switch dstV.Kind() {
	case reflect.Struct, reflect.Interface:
		hasValidateTag := false
		mapping := make(map[string]reflect.Value)
		fields, anonymous := extractFields(dstT)
		for _, anon := range anonymous {
			if field := dstV.FieldByName(anon.Name); field.Kind() == reflect.Ptr && field.IsNil() {
				field.Set(New(anon.Type.Elem()))
			}
		}
		for _, field := range fields {
			var key string
			if jsonTag, ok := field.Tag.Lookup(tagJSON); ok {
				if jsonTag == "-" {
					continue
				}
				key = strutils.CommaSeperatedList(jsonTag)[0]
			} else {
				key = field.Name
			}
			key = strutils.ToLowerNoSnake(key)
			mapping[key] = dstV.FieldByName(field.Name)

			if !hasValidateTag {
				_, hasValidateTag = field.Tag.Lookup(tagValidate)
			}

			aliases, ok := field.Tag.Lookup(tagAliases)
			if ok {
				for _, alias := range strutils.CommaSeperatedList(aliases) {
					mapping[alias] = dstV.FieldByName(field.Name)
				}
			}
		}
		for k, v := range src {
			if field, ok := mapping[strutils.ToLowerNoSnake(k)]; ok {
				err := Convert(reflect.ValueOf(v), field)
				if err != nil {
					errs.Add(err.Subject(k))
				}
			} else {
				errs.Add(ErrUnknownField.Subject(k).Withf(strutils.DoYouMean(NearestField(k, mapping))))
			}
		}
		if hasValidateTag {
			errs.Add(ValidateWithFieldTags(dstV.Interface()))
		}
		if err := ValidateWithCustomValidator(dstV); err != nil {
			errs.Add(err)
		}
		return errs.Error()
	case reflect.Map:
		for k, v := range src {
			mapVT := dstT.Elem()
			tmp := New(mapVT).Elem()
			err := Convert(reflect.ValueOf(v), tmp)
			if err != nil {
				errs.Add(err.Subject(k))
				continue
			}
			if err := ValidateWithCustomValidator(tmp.Addr()); err != nil {
				errs.Add(err.Subject(k))
			} else {
				dstV.SetMapIndex(reflect.ValueOf(k), tmp)
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

func isIntFloat(t reflect.Kind) bool {
	return t >= reflect.Bool && t <= reflect.Float64
}

func itoa(v reflect.Value) string {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	}
	panic("invalid call on itoa")
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
func Convert(src reflect.Value, dst reflect.Value) gperr.Error {
	if !dst.IsValid() {
		return gperr.Errorf("convert: dst is %w", ErrNilValue)
	}

	if !src.IsValid() {
		if dst.CanSet() {
			dst.Set(reflect.Zero(dst.Type()))
			return nil
		}
		return gperr.Errorf("convert: src is %w", ErrNilValue)
	}

	srcT := src.Type()
	dstT := dst.Type()

	if src.Kind() == reflect.Interface {
		src = src.Elem()
		srcT = src.Type()
	}

	if !dst.CanSet() {
		return ErrUnsettable.Subject(dstT.String())
	}

	if dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(New(dstT.Elem()))
		}
		dst = dst.Elem()
		dstT = dst.Type()
	}

	srcKind := srcT.Kind()

	switch {
	case srcT.AssignableTo(dstT):
		dst.Set(src)
		return nil
	case srcKind == reflect.String:
		if convertible, err := ConvertString(src.String(), dst); convertible {
			return err
		}
	case isIntFloat(srcKind):
		if dst.Kind() == reflect.String {
			dst.Set(reflect.ValueOf(itoa(src)))
			return nil
		}
		if dst.Addr().Type().Implements(typeStrParser) {
			return Convert(reflect.ValueOf(itoa(src)), dst)
		}
		if !isIntFloat(dstT.Kind()) || !src.CanConvert(dstT) {
			return ErrUnsupportedConversion.Subjectf("%s to %s", srcT, dstT)
		}
		dst.Set(src.Convert(dstT))
		return nil
	case srcKind == reflect.Map:
		if src.Len() == 0 {
			return nil
		}
		obj, ok := src.Interface().(SerializedObject)
		if !ok {
			return ErrUnsupportedConversion.Subjectf("%s to %s", srcT, dstT)
		}
		return MapUnmarshalValidate(obj, dst.Addr().Interface())
	case srcKind == reflect.Slice:
		if src.Len() == 0 {
			return nil
		}
		if dstT.Kind() != reflect.Slice {
			return ErrUnsupportedConversion.Subjectf("%s to %s", srcT, dstT)
		}
		sliceErrs := gperr.NewBuilder("slice conversion errors")
		newSlice := reflect.MakeSlice(dstT, src.Len(), src.Len())
		i := 0
		for j, v := range src.Seq2() {
			tmp := New(dstT.Elem()).Elem()
			err := Convert(v, tmp)
			if err != nil {
				sliceErrs.Add(err.Subjectf("[%d]", j))
				continue
			}
			newSlice.Index(i).Set(tmp)
			i++
		}
		if err := sliceErrs.Error(); err != nil {
			return err
		}
		dst.Set(newSlice)
		return nil
	}
	return ErrUnsupportedConversion.Subjectf("%s to %s", srcT, dstT)
}

func isSameOrEmbededType(src, dst reflect.Type) bool {
	return src == dst || src.ConvertibleTo(dst)
}

func setSameOrEmbedddType(src, dst reflect.Value) {
	dstT := dst.Type()
	if src.Type().AssignableTo(dstT) {
		dst.Set(src)
	} else {
		dst.Set(src.Convert(dstT))
	}
}

func ConvertString(src string, dst reflect.Value) (convertible bool, convErr gperr.Error) {
	convertible = true
	dstT := dst.Type()
	if dst.Kind() == reflect.Ptr {
		if dst.IsNil() {
			dst.Set(New(dstT.Elem()))
		}
		dst = dst.Elem()
		dstT = dst.Type()
	}
	dstKind := dst.Kind()
	if dstKind == reflect.String {
		dst.SetString(src)
		return
	}
	if src == "" {
		dst.Set(reflect.Zero(dstT))
		return
	}
	switch {
	case dstT == typeDuration:
		d, err := time.ParseDuration(src)
		if err != nil {
			return true, gperr.Wrap(err)
		}
		if d < 0 {
			return true, gperr.New("duration must be greater than 0 if defined")
		}
		dst.Set(reflect.ValueOf(d))
		return
	case isSameOrEmbededType(dstT, typeURL):
		u, err := url.Parse(src)
		if err != nil {
			return true, gperr.Wrap(err)
		}
		setSameOrEmbedddType(reflect.ValueOf(u).Elem(), dst)
		return
	case isSameOrEmbededType(dstT, typeCIDR):
		if !strings.ContainsRune(src, '/') {
			src += "/32" // single IP
		}
		_, ipnet, err := net.ParseCIDR(src)
		if err != nil {
			return true, gperr.Wrap(err)
		}
		setSameOrEmbedddType(reflect.ValueOf(ipnet).Elem(), dst)
		return
	}
	if isIntFloat(dstKind) {
		var i any
		var err error
		switch {
		case dstKind == reflect.Bool:
			i, err = strconv.ParseBool(src)
		case dst.CanInt():
			i, err = strconv.ParseInt(src, 10, dstT.Bits())
		case dst.CanUint():
			i, err = strconv.ParseUint(src, 10, dstT.Bits())
		case dst.CanFloat():
			i, err = strconv.ParseFloat(src, dstT.Bits())
		}
		if err != nil {
			// avoid long error message
			var numErr *strconv.NumError
			if errors.As(err, &numErr) {
				return true, gperr.Wrap(numErr.Err, dstT.Name()).Subject(src)
			}
			return true, gperr.Wrap(err).Subject(src)
		}
		dst.Set(reflect.ValueOf(i).Convert(dstT))
		return
	}
	// check if target implements string parser
	if parser, ok := dst.Addr().Interface().(strutils.Parser); ok {
		return true, gperr.Wrap(parser.Parse(src))
	}
	// yaml like
	var tmp any
	switch dstKind {
	case reflect.Map, reflect.Struct:
		rawMap := make(SerializedObject)
		err := yaml.Unmarshal([]byte(src), &rawMap)
		if err != nil {
			return true, gperr.Wrap(err)
		}
		tmp = rawMap
	case reflect.Slice:
		src = strings.TrimSpace(src)
		isMultiline := strings.ContainsRune(src, '\n')
		// treats one liner without leading dash as comma separated list
		if !isMultiline && src[0] != '-' {
			values := strutils.CommaSeperatedList(src)
			dst.Set(reflect.MakeSlice(dst.Type(), len(values), len(values)))
			errs := gperr.NewBuilder("invalid slice values")
			for i, v := range values {
				err := Convert(reflect.ValueOf(v), dst.Index(i))
				if err != nil {
					errs.Add(err.Subjectf("[%d]", i))
				}
			}
			if errs.HasError() {
				return true, errs.Error()
			}
			return
		}
		sl := make([]any, 0)
		err := yaml.Unmarshal([]byte(src), &sl)
		if err != nil {
			return true, gperr.Wrap(err)
		}
		tmp = sl
	default:
		return false, nil
	}
	return true, Convert(reflect.ValueOf(tmp), dst)
}

func UnmarshalValidateYAML[T any](data []byte, target *T) gperr.Error {
	m := make(map[string]any)
	if err := yaml.Unmarshal(data, &m); err != nil {
		return gperr.Wrap(err)
	}
	return MapUnmarshalValidate(m, target)
}

func UnmarshalValidateYAMLMap[V any](data []byte) (_ functional.Map[string, V], err gperr.Error) {
	m := make(map[string]any)
	if err = gperr.Wrap(yaml.Unmarshal(data, &m)); err != nil {
		return
	}
	m2 := make(map[string]V, len(m))
	if err = MapUnmarshalValidate(m, m2); err != nil {
		return
	}
	return functional.NewMapFrom(m2), nil
}

func loadSerialized[T any](path string, dst *T, deserialize func(data []byte, dst any) error) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return deserialize(data, dst)
}

func SaveJSON[T any](path string, src *T, perm os.FileMode) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func LoadJSONIfExist[T any](path string, dst *T) error {
	err := loadSerialized(path, dst, json.Unmarshal)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

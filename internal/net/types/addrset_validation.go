package nettypes

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var ErrUndefinedAddressSet = errors.New("address set is not defined")

// AddressSetValidator resolves references against one configuration's definitions.
// Keeping it explicit lets candidate validation run without publishing any state.
type AddressSetValidator func(name string) error

// PublishedAddressSetValidator validates standalone edits against the active sets.
func PublishedAddressSetValidator(name string) error {
	ref := AddressSet{name: name}
	if !ref.Defined() {
		return fmt.Errorf("%w: set:%s", ErrUndefinedAddressSet, name)
	}
	return nil
}

type addressSetValidatable interface {
	ValidateAddressSetReferences(AddressSetValidator) error
}

// ValidateAddressSetReferences walks configuration values, not runtime objects.
// Opaque parsed values expose their references through the validation method.
// Strings are deliberately ignored: a literal header or message is not a source.
func ValidateAddressSetReferences(value any, check AddressSetValidator) error {
	return validateAddressSetReferences(reflect.ValueOf(value), check)
}

func validateAddressSetReferences(v reflect.Value, check AddressSetValidator) error {
	if !v.IsValid() || ((v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil()) {
		return nil
	}
	if v.Kind() == reflect.Interface {
		return validateAddressSetReferences(v.Elem(), check)
	}
	if v.CanInterface() {
		if source, ok := v.Interface().(addressSetValidatable); ok {
			return source.ValidateAddressSetReferences(check)
		}
	}
	switch v.Kind() {
	case reflect.Pointer:
		return validateAddressSetReferences(v.Elem(), check)
	case reflect.Struct:
		for i := range v.NumField() {
			field := v.Type().Field(i)
			if !field.IsExported() || field.Tag.Get("json") == "-" {
				continue
			}
			if err := validateAddressSetReferences(v.Field(i), check); err != nil {
				name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
				if name == "" {
					name = field.Name
				}
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if err := validateAddressSetReferences(v.Index(i), check); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if err := validateAddressSetReferences(iter.Value(), check); err != nil {
				return fmt.Errorf("%v: %w", iter.Key(), err)
			}
		}
	}
	return nil
}

func (s *AddressSet) ValidateAddressSetReferences(check AddressSetValidator) error {
	return check(s.name)
}

func (cidr CIDR) ValidateAddressSetReferences(check AddressSetValidator) error {
	if cidr.set != nil {
		return check(cidr.set.name)
	}
	return nil
}

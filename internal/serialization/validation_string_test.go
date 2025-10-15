package serialization

import (
	"reflect"
	"testing"

	gperr "github.com/yusing/goutils/errs"
)

type CustomValidatingString string

func (c CustomValidatingString) Validate() gperr.Error {
	if c == "" {
		return gperr.New("string cannot be empty")
	}
	if len(c) < 2 {
		return gperr.New("string must be at least 2 characters")
	}
	return nil
}

func TestValidateWithCustomValidator_String(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"regular string - no custom validation", "hello", false},
		{"empty regular string - no custom validation", "", false},
		{"short regular string - no custom validation", "a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWithCustomValidator(reflect.ValueOf(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWithCustomValidator() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWithCustomValidator_CustomValidatingString(t *testing.T) {
	tests := []struct {
		name    string
		input   CustomValidatingString
		wantErr bool
	}{
		{"valid custom validating string", CustomValidatingString("hello"), false},
		{"invalid custom validating string - empty", CustomValidatingString(""), true},
		{"invalid custom validating string - too short", CustomValidatingString("a"), true},
		{"valid custom validating string - minimum length", CustomValidatingString("ab"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWithCustomValidator(reflect.ValueOf(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWithCustomValidator() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWithCustomValidator_CustomValidatingStringPointer(t *testing.T) {
	tests := []struct {
		name    string
		input   *CustomValidatingString
		wantErr bool
	}{
		{"valid custom validating string pointer", ptr(CustomValidatingString("hello")), false},
		{"nil custom validating string pointer", nil, true},
		{"invalid custom validating string pointer - empty", ptr(CustomValidatingString("")), true},
		{"invalid custom validating string pointer - too short", ptr(CustomValidatingString("a")), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWithCustomValidator(reflect.ValueOf(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWithCustomValidator() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

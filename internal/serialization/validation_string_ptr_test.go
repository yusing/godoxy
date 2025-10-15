package serialization

import (
	"reflect"
	"testing"

	gperr "github.com/yusing/goutils/errs"
)

type CustomValidatingPointerString string

func (c *CustomValidatingPointerString) Validate() gperr.Error {
	if c == nil {
		return gperr.New("pointer string cannot be nil")
	}
	if *c == "" {
		return gperr.New("string cannot be empty")
	}
	if len(*c) < 2 {
		return gperr.New("string must be at least 2 characters")
	}
	return nil
}

func TestValidateWithCustomValidator_StringPointer(t *testing.T) {
	tests := []struct {
		name    string
		input   *string
		wantErr bool
	}{
		{"valid string pointer", ptr("hello"), false},
		{"nil string pointer", nil, false},
		{"empty string pointer", ptr(""), false},
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

func TestValidateWithCustomValidator_CustomValidatingPointerStringValue(t *testing.T) {
	tests := []struct {
		name    string
		input   CustomValidatingPointerString
		wantErr bool
	}{
		{"custom validating pointer string as value - valid", CustomValidatingPointerString("hello"), false},
		{"custom validating pointer string as value - empty", CustomValidatingPointerString(""), false},
		{"custom validating pointer string as value - short", CustomValidatingPointerString("a"), false},
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

func TestValidateWithCustomValidator_CustomValidatingPointerStringPointer(t *testing.T) {
	tests := []struct {
		name    string
		input   *CustomValidatingPointerString
		wantErr bool
	}{
		{"valid custom validating pointer string", customStringPointerPtr(CustomValidatingPointerString("hello")), false},
		{"nil custom validating pointer string", nil, true}, // Should fail because Validate() checks for nil
		{"invalid custom validating pointer string - empty", customStringPointerPtr(CustomValidatingPointerString("")), true},
		{"invalid custom validating pointer string - too short", customStringPointerPtr(CustomValidatingPointerString("a")), true},
		{"valid custom validating pointer string - minimum length", customStringPointerPtr(CustomValidatingPointerString("ab")), false},
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

// Helper function to create CustomValidatingPointerString pointer
func customStringPointerPtr(s CustomValidatingPointerString) *CustomValidatingPointerString {
	return &s
}

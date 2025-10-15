package serialization

import (
	"reflect"
	"testing"

	gperr "github.com/yusing/goutils/errs"
)

type CustomValidatingPointerStruct struct {
	Value string
}

func (c *CustomValidatingPointerStruct) Validate() gperr.Error {
	if c == nil {
		return gperr.New("pointer struct cannot be nil")
	}
	if c.Value == "" {
		return gperr.New("value cannot be empty")
	}
	if len(c.Value) < 3 {
		return gperr.New("value must be at least 3 characters")
	}
	return nil
}

func TestValidateWithCustomValidator_CustomValidatingPointerStructValue(t *testing.T) {
	tests := []struct {
		name    string
		input   CustomValidatingPointerStruct
		wantErr bool
	}{
		{"custom validating pointer struct as value - valid", CustomValidatingPointerStruct{Value: "hello"}, false},
		{"custom validating pointer struct as value - empty", CustomValidatingPointerStruct{Value: ""}, false},
		{"custom validating pointer struct as value - short", CustomValidatingPointerStruct{Value: "hi"}, false},
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

func TestValidateWithCustomValidator_CustomValidatingPointerStructPointer(t *testing.T) {
	tests := []struct {
		name    string
		input   *CustomValidatingPointerStruct
		wantErr bool
	}{
		{"valid custom validating pointer struct", &CustomValidatingPointerStruct{Value: "hello"}, false},
		{"nil custom validating pointer struct", nil, true}, // Should fail because Validate() checks for nil
		{"invalid custom validating pointer struct - empty", &CustomValidatingPointerStruct{Value: ""}, true},
		{"invalid custom validating pointer struct - too short", &CustomValidatingPointerStruct{Value: "hi"}, true},
		{"valid custom validating pointer struct - minimum length", &CustomValidatingPointerStruct{Value: "abc"}, false},
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

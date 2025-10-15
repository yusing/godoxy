package serialization

import (
	"reflect"
	"testing"

	gperr "github.com/yusing/goutils/errs"
)

type CustomValidatingStruct struct {
	Value string
}

func (c CustomValidatingStruct) Validate() gperr.Error {
	if c.Value == "" {
		return gperr.New("value cannot be empty")
	}
	if len(c.Value) < 3 {
		return gperr.New("value must be at least 3 characters")
	}
	return nil
}

func TestValidateWithCustomValidator_Struct(t *testing.T) {
	tests := []struct {
		name    string
		input   CustomValidatingStruct
		wantErr bool
	}{
		{"valid custom validating struct", CustomValidatingStruct{Value: "hello"}, false},
		{"invalid custom validating struct - empty", CustomValidatingStruct{Value: ""}, true},
		{"invalid custom validating struct - too short", CustomValidatingStruct{Value: "hi"}, true},
		{"valid custom validating struct - minimum length", CustomValidatingStruct{Value: "abc"}, false},
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

func TestValidateWithCustomValidator_CustomValidatingStructPointer(t *testing.T) {
	tests := []struct {
		name    string
		input   *CustomValidatingStruct
		wantErr bool
	}{
		{"valid custom validating struct pointer", &CustomValidatingStruct{Value: "hello"}, false},
		{"nil custom validating struct pointer", nil, true},
		{"invalid custom validating struct pointer - empty", &CustomValidatingStruct{Value: ""}, true},
		{"invalid custom validating struct pointer - too short", &CustomValidatingStruct{Value: "hi"}, true},
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

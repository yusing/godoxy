package serialization

import (
	"reflect"
	"testing"

	gperr "github.com/yusing/goutils/errs"
)

// Test cases for when *T implements CustomValidator but T is passed in
type CustomValidatingInt int

func (c *CustomValidatingInt) Validate() gperr.Error {
	if c == nil {
		return gperr.New("pointer int cannot be nil")
	}
	if *c <= 0 {
		return gperr.New("int must be positive")
	}
	if *c > 100 {
		return gperr.New("int must be <= 100")
	}
	return nil
}

// Test cases for when T implements CustomValidator but *T is passed in
type CustomValidatingFloat float64

func (c CustomValidatingFloat) Validate() gperr.Error {
	if c < 0 {
		return gperr.New("float must be non-negative")
	}
	if c > 1000 {
		return gperr.New("float must be <= 1000")
	}
	return nil
}

func TestValidateWithCustomValidator_PointerMethodButValuePassed(t *testing.T) {
	tests := []struct {
		name    string
		input   CustomValidatingInt
		wantErr bool
	}{
		{"custom validating int as value - valid", CustomValidatingInt(50), false},
		{"custom validating int as value - zero", CustomValidatingInt(0), false},
		{"custom validating int as value - negative", CustomValidatingInt(-5), false},
		{"custom validating int as value - large", CustomValidatingInt(200), false},
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

func TestValidateWithCustomValidator_PointerMethodWithPointerPassed(t *testing.T) {
	tests := []struct {
		name    string
		input   *CustomValidatingInt
		wantErr bool
	}{
		{"valid custom validating int pointer", ptr(CustomValidatingInt(50)), false},
		{"nil custom validating int pointer", nil, true}, // Should fail because Validate() checks for nil
		{"invalid custom validating int pointer - zero", ptr(CustomValidatingInt(0)), true},
		{"invalid custom validating int pointer - negative", ptr(CustomValidatingInt(-5)), true},
		{"invalid custom validating int pointer - too large", ptr(CustomValidatingInt(200)), true},
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

func TestValidateWithCustomValidator_ValueMethodButPointerPassed(t *testing.T) {
	tests := []struct {
		name    string
		input   *CustomValidatingFloat
		wantErr bool
	}{
		{"valid custom validating float pointer", ptr(CustomValidatingFloat(50.5)), false},
		{"nil custom validating float pointer", nil, false},
		{"invalid custom validating float pointer - negative", ptr(CustomValidatingFloat(-5.5)), true},
		{"invalid custom validating float pointer - too large", ptr(CustomValidatingFloat(2000.5)), true},
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

func TestValidateWithCustomValidator_ValueMethodWithValuePassed(t *testing.T) {
	tests := []struct {
		name    string
		input   CustomValidatingFloat
		wantErr bool
	}{
		{"valid custom validating float", CustomValidatingFloat(50.5), false},
		{"invalid custom validating float - negative", CustomValidatingFloat(-5.5), true},
		{"invalid custom validating float - too large", CustomValidatingFloat(2000.5), true},
		{"valid custom validating float - boundary", CustomValidatingFloat(1000), false},
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

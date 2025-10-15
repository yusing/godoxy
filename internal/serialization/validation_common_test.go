package serialization

import (
	"testing"

	"github.com/go-playground/validator/v10"
)

// Common helper functions
func ptr[T any](s T) *T {
	return &s
}

// Common test function for MustRegisterValidation
func TestMustRegisterValidation(t *testing.T) {
	// Test registering a custom validation
	fn := func(fl validator.FieldLevel) bool {
		return fl.Field().String() != "invalid"
	}

	// This should not panic
	MustRegisterValidation("test_tag", fn)

	// Verify the validation was registered
	err := validate.VarWithValue("valid", "test", "test_tag")
	if err != nil {
		t.Errorf("Expected validation to pass, got error: %v", err)
	}

	err = validate.VarWithValue("invalid", "test", "test_tag")
	if err == nil {
		t.Error("Expected validation to fail")
	}
}

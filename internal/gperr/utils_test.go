package gperr

import (
	"errors"
	"testing"
)

type testErr struct{}

func (e *testErr) Error() string {
	return "test error"
}

func (e *testErr) MarshalJSON() ([]byte, error) {
	return nil, nil
}

func TestIsJSONMarshallable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "testErr",
			err:  &testErr{},
			want: true,
		},
		{
			name: "baseError",
			err:  &baseError{},
			want: false,
		},
		{
			name: "baseError with json marshallable error",
			err:  &baseError{&testErr{}},
			want: true,
		},
		{
			name: "nestedError",
			err:  &nestedError{},
			want: true,
		},
		{
			name: "withSubject",
			err:  &withSubject{},
			want: true,
		},
		{
			name: "standard error",
			err:  errors.New("test error"),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsJSONMarshallable(test.err); got != test.want {
				t.Errorf("IsJSONMarshallable(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}

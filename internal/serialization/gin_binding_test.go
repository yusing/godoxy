package serialization_test

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
)

type TestStruct struct {
	Value  string `json:"value"`
	Value2 int    `json:"value2"`
}

func (t *TestStruct) Validate() gperr.Error {
	if t.Value == "" {
		return gperr.New("value is required")
	}
	if t.Value2 != 0 && (t.Value2 < 5 || t.Value2 > 10) {
		return gperr.New("value2 must be between 5 and 10")
	}
	return nil
}

func TestGinBinding(t *testing.T) {

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid1", `{"value": "test", "value2": 7}`, false},
		{"valid2", `{"value": "test"}`, false},
		{"invalid1", `{"value2": 7}`, true},
		{"invalid2", `{"value": "test", "value2": 3}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst TestStruct
			body := bytes.NewBufferString(tt.input)
			req := httptest.NewRequest("POST", "/", body)
			err := serialization.GinJSONBinding{}.Bind(req, &dst)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s: Bind() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

package route

import (
	"errors"
	"strconv"
	"testing"
)

var invalidPorts = []string{
	"",
	"123:",
	"0:",
	"qwerty",
	"asdfgh:asdfgh",
	"1234:asdfgh",
}

var tooManyColonsPorts = []string{
	"1234:1234:1234",
}

var outOfRangePorts = []string{
	"-1:1234",
	"1234:-1",
	"65536",
	"0:65536",
}

func TestPortInvalid(t *testing.T) {
	tests := []struct {
		name    string
		inputs  []string
		wantErr error
	}{
		{
			name:    "invalid",
			inputs:  invalidPorts,
			wantErr: strconv.ErrSyntax,
		},

		{
			name:    "too many colons",
			inputs:  tooManyColonsPorts,
			wantErr: ErrInvalidPortSyntax,
		},
		{
			name:    "out of range",
			inputs:  outOfRangePorts,
			wantErr: ErrPortOutOfRange,
		},
	}

	for _, tc := range tests {
		for _, input := range tc.inputs {
			t.Run(tc.name, func(t *testing.T) {
				p := &Port{}
				err := p.Parse(input)
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("expected error %v, got %v", tc.wantErr, err)
				}
			})
		}
	}
}

func TestPortValid(t *testing.T) {
	tests := []struct {
		name   string
		inputs string
		expect Port
	}{
		{
			name:   "valid_lp",
			inputs: "1234:5678",
			expect: Port{
				Listening: 1234,
				Proxy:     5678,
			},
		},
		{
			name:   "valid_lp_empty",
			inputs: ":1234",
			expect: Port{
				Listening: 0,
				Proxy:     1234,
			},
		},
		{
			name:   "valid_p",
			inputs: "5678",
			expect: Port{
				Listening: 0,
				Proxy:     5678,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Port{}
			err := p.Parse(tc.inputs)
			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
			if p.Listening != tc.expect.Listening {
				t.Errorf("expected listening port %d, got %d", tc.expect.Listening, p.Listening)
			}
			if p.Proxy != tc.expect.Proxy {
				t.Errorf("expected proxy port %d, got %d", tc.expect.Proxy, p.Proxy)
			}
		})
	}
}

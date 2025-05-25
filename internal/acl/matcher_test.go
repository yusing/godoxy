package acl

import (
	"net"
	"reflect"
	"testing"

	maxmind "github.com/yusing/go-proxy/internal/maxmind/types"
	"github.com/yusing/go-proxy/internal/serialization"
)

func TestMatchers(t *testing.T) {
	strMatchers := []string{
		"ip:127.0.0.1",
		"cidr:10.0.0.0/8",
	}

	var mathers Matchers
	err := serialization.Convert(reflect.ValueOf(strMatchers), reflect.ValueOf(&mathers), false)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"127.0.0.2", false},
		{"192.168.0.1", false},
		{"11.0.0.1", false},
	}

	for _, test := range tests {
		ip := net.ParseIP(test.ip)
		if ip == nil {
			t.Fatalf("invalid ip: %s", test.ip)
		}

		got := mathers.Match(&maxmind.IPInfo{
			IP:  ip,
			Str: test.ip,
		})
		if got != test.want {
			t.Errorf("mathers.Match(%s) = %v, want %v", test.ip, got, test.want)
		}
	}
}

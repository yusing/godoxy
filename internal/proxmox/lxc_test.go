package proxmox

import (
	"net"
	"reflect"
	"testing"
)

func TestGetIPFromNet(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  []net.IP
	}{
		{
			name:  "ipv4 only",
			input: "name=eth0,bridge=vmbr0,gw=10.0.0.1,hwaddr=BC:24:11:10:88:97,ip=10.0.6.68/16,type=veth",
			want:  []net.IP{net.ParseIP("10.0.6.68")},
		},
		{
			name:  "ipv6 only, at the end",
			input: "name=eth0,bridge=vmbr0,hwaddr=BC:24:11:10:88:97,gw=::ffff:a00:1,type=veth,ip6=::ffff:a00:644/48",
			want:  []net.IP{net.ParseIP("::ffff:a00:644")},
		},
		{
			name:  "both",
			input: "name=eth0,bridge=vmbr0,hwaddr=BC:24:11:10:88:97,gw=::ffff:a00:1,type=veth,ip6=::ffff:a00:644/48,ip=10.0.6.68/16",
			want:  []net.IP{net.ParseIP("10.0.6.68"), net.ParseIP("::ffff:a00:644")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := getIPFromNet(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("getIPFromNet(%q) = %s, want %s", tc.name, got, tc.want)
			}
		})
	}
}

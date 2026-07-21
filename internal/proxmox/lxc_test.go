package proxmox

import (
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	goproxmox "github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
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

func TestLXCGetIPsFromConfigReadsAllIndexedNetworksInOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/nodes/pve/lxc/101/config", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{
			"net31":"name=eth31,ip=10.0.31.2/24",
			"net3":"name=eth3,ip=10.0.3.2/24",
			"net100":"name=eth100,ip=10.0.100.2/24"
		}}`))
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, goproxmox.WithHTTPClient(srv.Client()))
	node := NewNode(client, "pve", "node/pve")
	ips, err := node.LXCGetIPsFromConfig(t.Context(), 101)
	require.NoError(t, err)
	require.Len(t, ips, 3)
	require.Equal(t, []string{"10.0.3.2", "10.0.31.2", "10.0.100.2"}, []string{
		ips[0].String(),
		ips[1].String(),
		ips[2].String(),
	})
}

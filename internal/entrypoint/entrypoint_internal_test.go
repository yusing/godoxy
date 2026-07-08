package entrypoint

import "testing"

func TestHostWithoutPort(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{
			name: "hostname with port",
			host: "app.example.com:8080",
			want: "app.example.com",
		},
		{
			name: "ipv6 with port",
			host: "[::1]:8080",
			want: "::1",
		},
		{
			name: "hostname without port",
			host: "app.example.com",
			want: "app.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hostWithoutPort(tt.host); got != tt.want {
				t.Fatalf("hostWithoutPort(%q) = %q, want %q", tt.host, got, tt.want)
			}
		})
	}
}

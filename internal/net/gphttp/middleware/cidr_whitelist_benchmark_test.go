package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkCIDRWhitelist(b *testing.B) {
	m, err := CIDRWhiteList.New(OptionsRaw{
		"allow": []string{
			"127.0.0.1", "192.168.0.0/16",
		},
	})
	require.NoError(b, err)

	reqMatch := &http.Request{Host: "192.168.50.123:12345"}
	reqNoMatch := &http.Request{Host: "100.64.51.123:12345"}

	rw := http.ResponseWriter(noopRW{})
	wl := m.impl.(*cidrWhitelist)

	b.Run("match", func(b *testing.B) {
		for b.Loop() {
			wl.checkIP(rw, reqMatch)
		}
	})
	b.Run("no match", func(b *testing.B) {
		for b.Loop() {
			wl.checkIP(rw, reqNoMatch)
		}
	})
}

type noopRW struct{}

func (noopRW) Header() http.Header {
	return http.Header{}
}

func (noopRW) Write([]byte) (int, error) {
	return 0, nil
}

func (noopRW) WriteHeader(int) {}

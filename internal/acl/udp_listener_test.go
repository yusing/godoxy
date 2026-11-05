package acl

import (
	"errors"
	"io/fs"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/net/hostaddr"
)

type udpWriteRecorder struct {
	net.PacketConn
	writes int
	err    error
}

func TestUDPWriteRejectsRevokedHostAddress(t *testing.T) {
	records := map[string][]string{"home.example": {"203.0.113.7"}}
	hostaddr.UseStaticRecordsForTest(t, records)
	cfg := &Config{Default: ACLDeny, AllowLocal: new(false), Allow: mustMatchers(t, "host:home.example")}
	require.NoError(t, cfg.Validate())
	conn := &udpWriteRecorder{}
	wrapped := cfg.WrapUDP(conn)
	destination := &net.UDPAddr{IP: net.ParseIP("203.0.113.7"), Port: 1234}
	_, err := wrapped.WriteTo([]byte("response"), destination)
	require.NoError(t, err)
	require.Equal(t, 1, conn.writes)

	records["home.example"] = []string{"198.51.100.9"}
	hostaddr.RefreshAllForTest(t)
	n, err := wrapped.WriteTo([]byte("response"), destination)
	require.ErrorIs(t, err, fs.ErrPermission)
	require.Zero(t, n)
	require.Equal(t, 1, conn.writes, "revocation must take effect before sending")
}

func (c *udpWriteRecorder) WriteTo(p []byte, _ net.Addr) (int, error) {
	c.writes++
	if c.err != nil {
		return 0, c.err
	}
	return len(p), nil
}

func TestUDPWriteChecksDestinationBeforeSending(t *testing.T) {
	writeErr := errors.New("write failed")
	for _, tc := range []struct {
		name        string
		addr        net.Addr
		writeErr    error
		wantErr     error
		wantWrites  int
		wantWritten int
	}{
		{"allowed", &net.UDPAddr{IP: net.ParseIP("203.0.113.7")}, nil, nil, 1, 8},
		{"denied", &net.UDPAddr{IP: net.ParseIP("198.51.100.9")}, writeErr, fs.ErrPermission, 0, 0},
		{"invalid destination", &net.TCPAddr{}, writeErr, net.InvalidAddrError("not a UDP address"), 0, 0},
		{"write error", &net.UDPAddr{IP: net.ParseIP("203.0.113.7")}, writeErr, writeErr, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Default: ACLDeny, AllowLocal: new(false), Allow: mustMatchers(t, "ip:203.0.113.7")}
			require.NoError(t, cfg.Validate())
			conn := &udpWriteRecorder{err: tc.writeErr}
			n, err := cfg.WrapUDP(conn).WriteTo([]byte("response"), tc.addr)
			require.ErrorIs(t, err, tc.wantErr)
			require.Equal(t, tc.wantWritten, n)
			require.Equal(t, tc.wantWrites, conn.writes)
		})
	}
}

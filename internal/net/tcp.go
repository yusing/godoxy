package netutils

import (
	"context"
	"fmt"
	"net"
)

// PingTCP "pings" the IP address using TCP.
func PingTCP(ctx context.Context, ip net.IP, port int) error {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

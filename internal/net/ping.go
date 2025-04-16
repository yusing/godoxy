package netutils

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

var (
	ipv4EchoBytes []byte
	ipv6EchoBytes []byte
)

func init() {
	echoBody := &icmp.Echo{
		ID:   os.Getpid() & 0xffff,
		Seq:  1,
		Data: []byte("Hello"),
	}
	ipv4Echo := &icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Body: echoBody,
	}
	ipv6Echo := &icmp.Message{
		Type: ipv6.ICMPTypeEchoRequest,
		Body: echoBody,
	}
	var err error
	ipv4EchoBytes, err = ipv4Echo.Marshal(nil)
	if err != nil {
		panic(err)
	}
	ipv6EchoBytes, err = ipv6Echo.Marshal(nil)
	if err != nil {
		panic(err)
	}
}

// Ping pings the IP address using ICMP.
func Ping(ctx context.Context, ip net.IP) (bool, error) {
	var msgBytes []byte
	if ip.To4() != nil {
		msgBytes = ipv4EchoBytes
	} else {
		msgBytes = ipv6EchoBytes
	}

	conn, err := icmp.ListenPacket("ip:icmp", ip.String())
	if err != nil {
		return false, err
	}
	defer conn.Close()

	_, err = conn.WriteTo(msgBytes, &net.IPAddr{IP: ip})
	if err != nil {
		return false, err
	}

	err = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	if err != nil {
		return false, err
	}

	buf := make([]byte, 1500)
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return false, err
		}
		m, err := icmp.ParseMessage(ipv4.ICMPTypeEchoReply.Protocol(), buf[:n])
		if err != nil {
			continue
		}
		if m.Type == ipv4.ICMPTypeEchoReply {
			return true, nil
		}
	}
}

var pingDialer = &net.Dialer{
	Timeout: 1 * time.Second,
}

// PingWithTCPFallback pings the IP address using ICMP and TCP fallback.
//
// If the ICMP ping fails due to permission error, it will try to connect to the specified port.
func PingWithTCPFallback(ctx context.Context, ip net.IP, port int) (bool, error) {
	ok, err := Ping(ctx, ip)
	if err != nil {
		if !errors.Is(err, os.ErrPermission) {
			return false, err
		}
	} else {
		return ok, nil
	}

	conn, err := pingDialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return false, err
	}
	defer conn.Close()
	return true, nil
}

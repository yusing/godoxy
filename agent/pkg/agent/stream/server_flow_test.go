package stream

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

func newSerial(t *testing.T) *big.Int {
	t.Helper()
	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("rand serial: %v", err)
	}
	return sn
}

func genCA(t *testing.T) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          newSerial(t),
		Subject:               pkix.Name{CommonName: "stream-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate(CA): %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate(CA): %v", err)
	}
	return cert, key
}

func genLeafCert(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, cn string, eku x509.ExtKeyUsage) *tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: newSerial(t),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{eku},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(%s): %v", cn, err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate(%s): %v", cn, err)
	}
	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

func startTCPEcho(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen tcp: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(c)
		}
	}()

	return ln.Addr().String(), func() {
		_ = ln.Close()
		<-done
	}
}

func startUDPEcho(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen udp: %v", err)
	}
	uc := pc.(*net.UDPConn)

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 65535)
		for {
			n, raddr, err := uc.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = uc.WriteToUDP(buf[:n], raddr)
		}
	}()

	return uc.LocalAddr().String(), func() {
		_ = uc.Close()
		<-done
	}
}

func TestTCPServer_FullFlow(t *testing.T) {
	ca, caKey := genCA(t)
	serverCert := genLeafCert(t, ca, caKey, "stream-server", x509.ExtKeyUsageServerAuth)
	clientCert := genLeafCert(t, ca, caKey, "stream-client", x509.ExtKeyUsageClientAuth)

	dstAddr, closeDst := startTCPEcho(t)
	defer closeDst()

	tcpLn, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("ListenTCP: %v", err)
	}
	defer tcpLn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewTCPServer(ctx, tcpLn, ca, serverCert)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		cancel()
		_ = srv.Close()
		_ = <-errCh
	}()

	client, err := NewTCPClient(srv.Addr().String(), dstAddr, ca, clientCert)
	if err != nil {
		t.Fatalf("NewTCPClient: %v", err)
	}
	defer client.Close()

	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	msg := []byte("ping over tcp")
	if _, err := client.Write(msg); err != nil {
		t.Fatalf("client.Write: %v", err)
	}

	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("client.ReadFull: %v", err)
	}
	if string(buf) != string(msg) {
		t.Fatalf("unexpected echo: got %q want %q", string(buf), string(msg))
	}
}

func TestUDPServer_FullFlow(t *testing.T) {
	ca, caKey := genCA(t)
	serverCert := genLeafCert(t, ca, caKey, "stream-server", x509.ExtKeyUsageServerAuth)
	clientCert := genLeafCert(t, ca, caKey, "stream-client", x509.ExtKeyUsageClientAuth)

	dstAddr, closeDst := startUDPEcho(t)
	defer closeDst()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewUDPServer(ctx, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}, ca, serverCert)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		cancel()
		_ = srv.Close()
		err := <-errCh
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
			t.Logf("udp server exit: %v", err)
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for srv.listener == nil {
		if time.Now().After(deadline) {
			t.Fatalf("udp server listener did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	client, err := NewUDPClient(srv.Addr().String(), dstAddr, ca, clientCert)
	if err != nil {
		t.Fatalf("NewUDPClient: %v", err)
	}
	defer client.Close()

	_ = client.SetDeadline(time.Now().Add(2 * time.Second))
	msg := []byte("ping over udp")
	if _, err := client.Write(msg); err != nil {
		t.Fatalf("client.Write: %v", err)
	}

	buf := make([]byte, 2048)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("client.Read: %v", err)
	}
	if string(buf[:n]) != string(msg) {
		t.Fatalf("unexpected echo: got %q want %q", string(buf[:n]), string(msg))
	}
}

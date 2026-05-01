package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	tcpAddr = ":9000"
	tlsAddr = ":9443"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cert, err := newSelfSignedCert()
	if err != nil {
		log.Fatalf("generate tls cert: %v", err)
	}

	listeners := []struct {
		name string
		ln   net.Listener
	}{
		{name: "tcp", ln: mustListen("tcp", tcpAddr)},
		{name: "tls", ln: mustListenTLS(tlsAddr, cert)},
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(listeners))

	for _, listener := range listeners {
		wg.Go(func() {
			errCh <- serve(ctx, listener.name, listener.ln)
		})
	}

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			log.Printf("server error: %v", err)
		}
		stop()
	}

	for _, listener := range listeners {
		_ = listener.ln.Close()
	}
	wg.Wait()
}

func mustListen(network, addr string) net.Listener {
	ln, err := net.Listen(network, addr)
	if err != nil {
		log.Fatalf("listen %s %s: %v", network, addr, err)
	}
	log.Printf("tcp echo listening on %s", addr)
	return ln
}

func mustListenTLS(addr string, cert tls.Certificate) net.Listener {
	ln, err := tls.Listen("tcp", addr, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		log.Fatalf("listen tls %s: %v", addr, err)
	}
	log.Printf("tls echo listening on %s", addr)
	return ln
}

func serve(ctx context.Context, name string, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		go echo(name, conn)
	}
}

func echo(name string, conn net.Conn) {
	defer conn.Close()
	log.Printf("%s echo connection from %s", name, conn.RemoteAddr())
	if _, err := io.Copy(conn, conn); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Printf("%s echo error from %s: %v", name, conn.RemoteAddr(), err)
	}
}

func newSelfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "godoxy tcp echo dev cert",
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "tcp-echo-passthrough.my.app"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return tls.X509KeyPair(certPEM, keyPEM)
}

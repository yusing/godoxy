package autocert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func createTLSCert(dnsNames []string) (*tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, err
	}

	cn := ""
	if len(dnsNames) > 0 {
		cn = dnsNames[0]
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}

	return &tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}, nil
}

func BenchmarkSNIMatcher(b *testing.B) {
	matcher := sniMatcher{}

	wildcard1Cert, err := createTLSCert([]string{"*.example.com"})
	if err != nil {
		b.Fatal(err)
	}
	wildcard1 := &Provider{tlsCert: wildcard1Cert}

	wildcard2Cert, err := createTLSCert([]string{"*.test.com"})
	if err != nil {
		b.Fatal(err)
	}
	wildcard2 := &Provider{tlsCert: wildcard2Cert}

	wildcard3Cert, err := createTLSCert([]string{"*.foo.com"})
	if err != nil {
		b.Fatal(err)
	}
	wildcard3 := &Provider{tlsCert: wildcard3Cert}

	exact1Cert, err := createTLSCert([]string{"bar.example.com"})
	if err != nil {
		b.Fatal(err)
	}
	exact1 := &Provider{tlsCert: exact1Cert}

	exact2Cert, err := createTLSCert([]string{"baz.test.com"})
	if err != nil {
		b.Fatal(err)
	}
	exact2 := &Provider{tlsCert: exact2Cert}

	matcher.addProvider(wildcard1)
	matcher.addProvider(wildcard2)
	matcher.addProvider(wildcard3)
	matcher.addProvider(exact1)
	matcher.addProvider(exact2)

	b.Run("MatchWildcard", func(b *testing.B) {
		for b.Loop() {
			_ = matcher.match("sub.example.com")
		}
	})

	b.Run("MatchExact", func(b *testing.B) {
		for b.Loop() {
			_ = matcher.match("bar.example.com")
		}
	})
}

package agent

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"time"
)

const (
	CertsDNSName = "godoxy.agent"
	KeySize      = 2048
)

func toPEMPair(certDER []byte, key *rsa.PrivateKey) *PEMPair {
	return &PEMPair{
		Cert: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		Key:  pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
	}
}

func b64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func b64Decode(data string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(data)
}

type PEMPair struct {
	Cert, Key []byte
}

func (p *PEMPair) String() string {
	return b64Encode(p.Cert) + ";" + b64Encode(p.Key)
}

func (p *PEMPair) Load(data string) (err error) {
	parts := strings.Split(data, ";")
	if len(parts) != 2 {
		return errors.New("invalid PEM pair")
	}
	p.Cert, err = b64Decode(parts[0])
	if err != nil {
		return err
	}
	p.Key, err = b64Decode(parts[1])
	if err != nil {
		return err
	}
	return nil
}

func (p *PEMPair) ToTLSCert() (*tls.Certificate, error) {
	cert, err := tls.X509KeyPair(p.Cert, p.Key)
	return &cert, err
}

func NewAgent() (ca, srv, client *PEMPair, err error) {
	// Create the CA's certificate
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"GoDoxy"},
			CommonName:   CertsDNSName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1000, 0, 0), // 1000 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	caKey, err := rsa.GenerateKey(rand.Reader, KeySize)
	if err != nil {
		return nil, nil, nil, err
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	ca = toPEMPair(caDER, caKey)

	// Generate a new private key for the server certificate
	serverKey, err := rsa.GenerateKey(rand.Reader, KeySize)
	if err != nil {
		return nil, nil, nil, err
	}

	srvTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Issuer:       caTemplate.Subject,
		Subject:      caTemplate.Subject,
		DNSNames:     []string{CertsDNSName},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(1000, 0, 0), // Add validity period
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	srvCertDER, err := x509.CreateCertificate(rand.Reader, srvTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	srv = toPEMPair(srvCertDER, serverKey)

	clientKey, err := rsa.GenerateKey(rand.Reader, KeySize)
	if err != nil {
		return nil, nil, nil, err
	}

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Issuer:       caTemplate.Subject,
		Subject:      caTemplate.Subject,
		DNSNames:     []string{CertsDNSName},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(1000, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	client = toPEMPair(clientCertDER, clientKey)
	return
}

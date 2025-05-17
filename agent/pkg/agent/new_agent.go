package agent

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"time"

	"crypto/ecdsa"
	"crypto/elliptic"
	"fmt"
)

const (
	CertsDNSName = "godoxy.agent"
)

func toPEMPair(certDER []byte, key *ecdsa.PrivateKey) *PEMPair {
	marshaledKey, err := marshalECPrivateKey(key)
	if err != nil {
		// This is a critical internal error during PEM encoding of a newly generated key.
		// Panicking is acceptable here as it indicates a fundamental issue.
		panic(fmt.Sprintf("failed to marshal EC private key for PEM encoding: %v", err))
	}
	return &PEMPair{
		Cert: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		Key:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: marshaledKey}),
	}
}

func marshalECPrivateKey(key *ecdsa.PrivateKey) ([]byte, error) {
	derBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal EC private key: %w", err)
	}
	return derBytes, nil
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

func newSerialNumber() (*big.Int, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128) // 128-bit random number
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}
	return serialNumber, nil
}

func NewAgent() (ca, srv, client *PEMPair, err error) {
	caSerialNumber, err := newSerialNumber()
	if err != nil {
		return nil, nil, nil, err
	}
	// Create the CA's certificate
	caTemplate := &x509.Certificate{
		SerialNumber: caSerialNumber,
		Subject: pkix.Name{
			Organization: []string{"GoDoxy"},
			CommonName:   CertsDNSName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1000, 0, 0), // 1000 years
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		SignatureAlgorithm:    x509.ECDSAWithSHA256,
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	ca = toPEMPair(caDER, caKey)

	// Generate a new private key for the server certificate
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}

	serverSerialNumber, err := newSerialNumber()
	if err != nil {
		return nil, nil, nil, err
	}
	srvTemplate := &x509.Certificate{
		SerialNumber: serverSerialNumber,
		Issuer:       caTemplate.Subject,
		Subject: pkix.Name{
			Organization:       caTemplate.Subject.Organization,
			OrganizationalUnit: []string{"Server"},
			CommonName:         CertsDNSName,
		},
		DNSNames:           []string{CertsDNSName},
		NotBefore:          time.Now(),
		NotAfter:           time.Now().AddDate(1000, 0, 0), // Add validity period
		KeyUsage:           x509.KeyUsageDigitalSignature,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}

	srvCertDER, err := x509.CreateCertificate(rand.Reader, srvTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	srv = toPEMPair(srvCertDER, serverKey)

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}

	clientSerialNumber, err := newSerialNumber()
	if err != nil {
		return nil, nil, nil, err
	}
	clientTemplate := &x509.Certificate{
		SerialNumber: clientSerialNumber,
		Issuer:       caTemplate.Subject,
		Subject: pkix.Name{
			Organization:       caTemplate.Subject.Organization,
			OrganizationalUnit: []string{"Client"},
			CommonName:         CertsDNSName,
		},
		DNSNames:           []string{CertsDNSName},
		NotBefore:          time.Now(),
		NotAfter:           time.Now().AddDate(1000, 0, 0),
		KeyUsage:           x509.KeyUsageDigitalSignature,
		ExtKeyUsage:        []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}
	clientCertDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}

	client = toPEMPair(clientCertDER, clientKey)
	return
}

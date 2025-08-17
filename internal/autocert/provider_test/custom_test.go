//nolint:errchkjson,errcheck
package provider_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/go-proxy/internal/autocert"
	"github.com/yusing/go-proxy/internal/dnsproviders"
)

func TestMain(m *testing.M) {
	dnsproviders.InitProviders()
	m.Run()
}

func TestCustomProvider(t *testing.T) {
	t.Run("valid custom provider with step-ca", func(t *testing.T) {
		cfg := &autocert.Config{
			Email:       "test@example.com",
			Domains:     []string{"example.com", "*.example.com"},
			Provider:    autocert.ProviderCustom,
			CADirURL:    "https://ca.example.com:9000/acme/acme/directory",
			CertPath:    "certs/custom.crt",
			KeyPath:     "certs/custom.key",
			ACMEKeyPath: "certs/custom-acme.key",
		}

		err := cfg.Validate()
		require.NoError(t, err)

		user, legoCfg, err := cfg.GetLegoConfig()
		require.NoError(t, err)
		require.NotNil(t, user)
		require.NotNil(t, legoCfg)
		require.Equal(t, "https://ca.example.com:9000/acme/acme/directory", legoCfg.CADirURL)
		require.Equal(t, "test@example.com", user.Email)
	})

	t.Run("custom provider missing CADirURL", func(t *testing.T) {
		cfg := &autocert.Config{
			Email:    "test@example.com",
			Domains:  []string{"example.com"},
			Provider: autocert.ProviderCustom,
			// CADirURL is missing
		}

		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing field 'ca_dir_url'")
	})

	t.Run("custom provider with step-ca internal CA", func(t *testing.T) {
		cfg := &autocert.Config{
			Email:       "admin@internal.com",
			Domains:     []string{"internal.example.com", "api.internal.example.com"},
			Provider:    autocert.ProviderCustom,
			CADirURL:    "https://step-ca.internal:443/acme/acme/directory",
			CertPath:    "certs/internal.crt",
			KeyPath:     "certs/internal.key",
			ACMEKeyPath: "certs/internal-acme.key",
		}

		err := cfg.Validate()
		require.NoError(t, err)

		user, legoCfg, err := cfg.GetLegoConfig()
		require.NoError(t, err)
		require.NotNil(t, user)
		require.NotNil(t, legoCfg)
		require.Equal(t, "https://step-ca.internal:443/acme/acme/directory", legoCfg.CADirURL)
		require.Equal(t, "admin@internal.com", user.Email)

		provider := autocert.NewProvider(cfg, user, legoCfg)
		require.NotNil(t, provider)
		require.Equal(t, autocert.ProviderCustom, provider.GetName())
		require.Equal(t, "certs/internal.crt", provider.GetCertPath())
		require.Equal(t, "certs/internal.key", provider.GetKeyPath())
	})
}

func TestObtainCertFromCustomProvider(t *testing.T) {
	// Create a test ACME server
	acmeServer := newTestACMEServer(t)
	defer acmeServer.Close()

	t.Run("obtain cert from custom step-ca server", func(t *testing.T) {
		cfg := &autocert.Config{
			Email:       "test@example.com",
			Domains:     []string{"test.example.com"},
			Provider:    autocert.ProviderCustom,
			CADirURL:    acmeServer.URL() + "/acme/acme/directory",
			CertPath:    "certs/stepca-test.crt",
			KeyPath:     "certs/stepca-test.key",
			ACMEKeyPath: "certs/stepca-test-acme.key",
			HTTPClient:  acmeServer.httpClient(),
		}

		err := error(cfg.Validate())
		require.NoError(t, err)

		user, legoCfg, err := cfg.GetLegoConfig()
		require.NoError(t, err)
		require.NotNil(t, user)
		require.NotNil(t, legoCfg)

		provider := autocert.NewProvider(cfg, user, legoCfg)
		require.NotNil(t, provider)

		// Test obtaining certificate
		err = provider.ObtainCert()
		require.NoError(t, err)

		// Verify certificate was obtained
		cert, err := provider.GetCert(nil)
		require.NoError(t, err)
		require.NotNil(t, cert)

		// Verify certificate properties
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, x509Cert.DNSNames, "test.example.com")
		require.True(t, time.Now().Before(x509Cert.NotAfter))
		require.True(t, time.Now().After(x509Cert.NotBefore))
	})

	t.Run("obtain cert with EAB from custom step-ca server", func(t *testing.T) {
		cfg := &autocert.Config{
			Email:       "test@example.com",
			Domains:     []string{"test.example.com"},
			Provider:    autocert.ProviderCustom,
			CADirURL:    acmeServer.URL() + "/acme/acme/directory",
			CertPath:    "certs/stepca-eab-test.crt",
			KeyPath:     "certs/stepca-eab-test.key",
			ACMEKeyPath: "certs/stepca-eab-test-acme.key",
			HTTPClient:  acmeServer.httpClient(),
			EABKid:      "kid-123",
			EABHmac:     base64.RawURLEncoding.EncodeToString([]byte("secret")),
		}

		err := error(cfg.Validate())
		require.NoError(t, err)

		user, legoCfg, err := cfg.GetLegoConfig()
		require.NoError(t, err)
		require.NotNil(t, user)
		require.NotNil(t, legoCfg)

		provider := autocert.NewProvider(cfg, user, legoCfg)
		require.NotNil(t, provider)

		err = provider.ObtainCert()
		require.NoError(t, err)

		cert, err := provider.GetCert(nil)
		require.NoError(t, err)
		require.NotNil(t, cert)

		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		require.NoError(t, err)
		require.Contains(t, x509Cert.DNSNames, "test.example.com")
		require.True(t, time.Now().Before(x509Cert.NotAfter))
		require.True(t, time.Now().After(x509Cert.NotBefore))
	})
}

// testACMEServer implements a minimal ACME server for testing.
type testACMEServer struct {
	server     *httptest.Server
	caCert     *x509.Certificate
	caKey      *rsa.PrivateKey
	clientCSRs map[string]*x509.CertificateRequest
	orderID    string
}

func newTestACMEServer(t *testing.T) *testACMEServer {
	t.Helper()

	// Generate CA certificate and key
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test CA"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCert, err := x509.ParseCertificate(caCertDER)
	require.NoError(t, err)

	acme := &testACMEServer{
		caCert:     caCert,
		caKey:      caKey,
		clientCSRs: make(map[string]*x509.CertificateRequest),
		orderID:    "test-order-123",
	}

	mux := http.NewServeMux()
	acme.setupRoutes(mux)

	acme.server = httptest.NewUnstartedServer(mux)
	acme.server.TLS = &tls.Config{
		Certificates: []tls.Certificate{
			{
				Certificate: [][]byte{caCert.Raw},
				PrivateKey:  caKey,
			},
		},
		MinVersion: tls.VersionTLS12,
	}
	acme.server.StartTLS()
	return acme
}

func (s *testACMEServer) Close() {
	s.server.Close()
}

func (s *testACMEServer) URL() string {
	return s.server.URL
}

func (s *testACMEServer) httpClient() *http.Client {
	certPool := x509.NewCertPool()
	certPool.AddCert(s.caCert)

	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			TLSClientConfig: &tls.Config{
				RootCAs:    certPool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}
}

func (s *testACMEServer) setupRoutes(mux *http.ServeMux) {
	// ACME directory endpoint
	mux.HandleFunc("/acme/acme/directory", s.handleDirectory)

	// ACME endpoints
	mux.HandleFunc("/acme/new-nonce", s.handleNewNonce)
	mux.HandleFunc("/acme/new-account", s.handleNewAccount)
	mux.HandleFunc("/acme/new-order", s.handleNewOrder)
	mux.HandleFunc("/acme/authz/", s.handleAuthorization)
	mux.HandleFunc("/acme/chall/", s.handleChallenge)
	mux.HandleFunc("/acme/order/", s.handleOrder)
	mux.HandleFunc("/acme/cert/", s.handleCertificate)
}

func (s *testACMEServer) handleDirectory(w http.ResponseWriter, r *http.Request) {
	directory := map[string]interface{}{
		"newNonce":   s.server.URL + "/acme/new-nonce",
		"newAccount": s.server.URL + "/acme/new-account",
		"newOrder":   s.server.URL + "/acme/new-order",
		"keyChange":  s.server.URL + "/acme/key-change",
		"meta": map[string]interface{}{
			"termsOfService": s.server.URL + "/terms",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(directory)
}

func (s *testACMEServer) handleNewNonce(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Replay-Nonce", "test-nonce-12345")
	w.WriteHeader(http.StatusOK)
}

func (s *testACMEServer) handleNewAccount(w http.ResponseWriter, r *http.Request) {
	account := map[string]interface{}{
		"status":  "valid",
		"contact": []string{"mailto:test@example.com"},
		"orders":  s.server.URL + "/acme/orders",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", s.server.URL+"/acme/account/1")
	w.Header().Set("Replay-Nonce", "test-nonce-67890")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(account)
}

func (s *testACMEServer) handleNewOrder(w http.ResponseWriter, r *http.Request) {
	authzID := "test-authz-456"

	order := map[string]interface{}{
		"status":         "ready", // Skip pending state for simplicity
		"expires":        time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"identifiers":    []map[string]string{{"type": "dns", "value": "test.example.com"}},
		"authorizations": []string{s.server.URL + "/acme/authz/" + authzID},
		"finalize":       s.server.URL + "/acme/order/" + s.orderID + "/finalize",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", s.server.URL+"/acme/order/"+s.orderID)
	w.Header().Set("Replay-Nonce", "test-nonce-order")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(order)
}

func (s *testACMEServer) handleAuthorization(w http.ResponseWriter, r *http.Request) {
	authz := map[string]interface{}{
		"status":     "valid", // Skip challenge validation for simplicity
		"expires":    time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"identifier": map[string]string{"type": "dns", "value": "test.example.com"},
		"challenges": []map[string]interface{}{
			{
				"type":   "dns-01",
				"status": "valid",
				"url":    s.server.URL + "/acme/chall/test-chall-789",
				"token":  "test-token-abc123",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Replay-Nonce", "test-nonce-authz")
	json.NewEncoder(w).Encode(authz)
}

func (s *testACMEServer) handleChallenge(w http.ResponseWriter, r *http.Request) {
	challenge := map[string]interface{}{
		"type":   "dns-01",
		"status": "valid",
		"url":    r.URL.String(),
		"token":  "test-token-abc123",
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Replay-Nonce", "test-nonce-chall")
	json.NewEncoder(w).Encode(challenge)
}

func (s *testACMEServer) handleOrder(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/finalize") {
		s.handleFinalize(w, r)
		return
	}

	certURL := s.server.URL + "/acme/cert/" + s.orderID
	order := map[string]interface{}{
		"status":      "valid",
		"expires":     time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"identifiers": []map[string]string{{"type": "dns", "value": "test.example.com"}},
		"certificate": certURL,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Replay-Nonce", "test-nonce-order-get")
	json.NewEncoder(w).Encode(order)
}

func (s *testACMEServer) handleFinalize(w http.ResponseWriter, r *http.Request) {
	// Read the JWS payload
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}

	// Extract CSR from JWS payload
	csr, err := s.extractCSRFromJWS(body)
	if err != nil {
		http.Error(w, "Invalid CSR: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Store the CSR for certificate generation
	s.clientCSRs[s.orderID] = csr

	certURL := s.server.URL + "/acme/cert/" + s.orderID
	order := map[string]interface{}{
		"status":      "valid",
		"expires":     time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"identifiers": []map[string]string{{"type": "dns", "value": "test.example.com"}},
		"certificate": certURL,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", strings.TrimSuffix(r.URL.String(), "/finalize"))
	w.Header().Set("Replay-Nonce", "test-nonce-finalize")
	json.NewEncoder(w).Encode(order)
}

func (s *testACMEServer) extractCSRFromJWS(jwsData []byte) (*x509.CertificateRequest, error) {
	// Parse the JWS structure
	var jws struct {
		Protected string `json:"protected"`
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}

	if err := json.Unmarshal(jwsData, &jws); err != nil {
		return nil, err
	}

	// Decode the payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(jws.Payload)
	if err != nil {
		return nil, err
	}

	// Parse the finalize request
	var finalizeReq struct {
		CSR string `json:"csr"`
	}

	if err := json.Unmarshal(payloadBytes, &finalizeReq); err != nil {
		return nil, err
	}

	// Decode the CSR
	csrBytes, err := base64.RawURLEncoding.DecodeString(finalizeReq.CSR)
	if err != nil {
		return nil, err
	}

	// Parse the CSR
	csr, err := x509.ParseCertificateRequest(csrBytes)
	if err != nil {
		return nil, err
	}

	return csr, nil
}

func (s *testACMEServer) handleCertificate(w http.ResponseWriter, r *http.Request) {
	// Extract order ID from URL
	orderID := strings.TrimPrefix(r.URL.Path, "/acme/cert/")

	// Get the CSR for this order
	csr, exists := s.clientCSRs[orderID]
	if !exists {
		http.Error(w, "No CSR found for order", http.StatusBadRequest)
		return
	}

	// Create certificate using the public key from the client's CSR
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization: []string{"Test Cert"},
			Country:      []string{"US"},
		},
		DNSNames:              csr.DNSNames,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Use the public key from the CSR and sign with CA key
	certDER, err := x509.CreateCertificate(rand.Reader, template, s.caCert, csr.PublicKey, s.caKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return certificate chain
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.caCert.Raw})

	w.Header().Set("Content-Type", "application/pem-certificate-chain")
	w.Header().Set("Replay-Nonce", "test-nonce-cert")
	w.Write(append(certPEM, caPEM...))
}

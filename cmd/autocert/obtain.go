package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/autocert"
)

func obtainCert(cfg *autocert.Config) error {
	user, legoCfg, err := getLegoConfig(cfg)
	if err != nil {
		return err
	}

	client, err := lego.NewClient(legoCfg)
	if err != nil {
		return err
	}

	provider, err := challengeProvider(cfg)
	if err != nil {
		return err
	}
	if err := client.Challenge.SetDNS01Provider(
		provider,
		dns01.CondOption(len(cfg.Resolvers) > 0, dns01.AddRecursiveNameservers(cfg.Resolvers)),
	); err != nil {
		return err
	}

	if user.Registration == nil {
		if reg, err := client.Registration.ResolveAccountByKey(); err == nil {
			user.Registration = reg
			log.Info().Msg("reused acme registration from private key")
		} else if err := register(client, cfg, user); err != nil {
			return err
		}
	}

	cert, err := renewExistingCert(client, cfg)
	if err != nil {
		log.Err(err).Msg("cert renew failed, fallback to obtain")
	}
	if cert == nil {
		cert, err = client.Certificate.Obtain(certificate.ObtainRequest{
			Domains: cfg.Domains,
			Bundle:  true,
		})
		if err != nil {
			return err
		}
	}
	return saveCert(cfg, cert)
}

func getLegoConfig(cfg *autocert.Config) (*autocert.User, *lego.Config, error) {
	privKey, err := loadOrCreateACMEKey(cfg)
	if err != nil {
		return nil, nil, err
	}

	user := &autocert.User{Email: cfg.Email, Key: privKey}
	legoCfg := lego.NewConfig(user)

	keyType, err := autocert.ParseCertificateKeyType(cfg.CertificateKeyType)
	if err != nil {
		return nil, nil, err
	}
	legoCfg.Certificate.KeyType = keyType

	if cfg.HTTPClient != nil {
		clone := *cfg.HTTPClient
		if transport, ok := cfg.HTTPClient.Transport.(*http.Transport); ok && transport != nil {
			clone.Transport = transport.Clone()
		}
		legoCfg.HTTPClient = &clone
	}

	if cfg.CADirURL != "" {
		legoCfg.CADirURL = cfg.CADirURL
	}
	if err := configureCustomCACerts(legoCfg, cfg.CACerts); err != nil {
		return nil, nil, err
	}
	return user, legoCfg, nil
}

func loadOrCreateACMEKey(cfg *autocert.Config) (*ecdsa.PrivateKey, error) {
	if cfg.Provider == autocert.ProviderLocal || cfg.Provider == autocert.ProviderPseudo {
		return nil, nil
	}

	privKey, err := cfg.LoadACMEKey()
	if err == nil {
		return privKey, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("load ACME key: %w", err)
	}

	log.Info().Err(err).Msg("failed to load ACME private key, generating a new one")
	privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ACME private key: %w", err)
	}
	if err := cfg.SaveACMEKey(privKey); err != nil {
		return nil, fmt.Errorf("save ACME private key: %w", err)
	}
	return privKey, nil
}

func configureCustomCACerts(legoCfg *lego.Config, caCerts []string) error {
	if len(caCerts) == 0 {
		return nil
	}

	certPool, err := lego.CreateCertPool(caCerts, true)
	if err != nil {
		return fmt.Errorf("failed to create cert pool: %w", err)
	}
	rt, err := ensureHTTPTransportForTLS(legoCfg.HTTPClient)
	if err != nil {
		return err
	}

	tlsCfg := rt.TLSClientConfig
	if tlsCfg != nil {
		tlsCfg = tlsCfg.Clone()
	} else {
		tlsCfg = &tls.Config{}
	}
	if tlsCfg.MinVersion == 0 || tlsCfg.MinVersion < tls.VersionTLS12 {
		tlsCfg.MinVersion = tls.VersionTLS12
	}
	tlsCfg.RootCAs = certPool
	rt.TLSClientConfig = tlsCfg
	legoCfg.HTTPClient.Transport = rt
	return nil
}

func challengeProvider(cfg *autocert.Config) (challenge.Provider, error) {
	providerName := cfg.Provider
	if providerName == autocert.ProviderCustom {
		providerName = autocert.ProviderLocal
	}
	providerConstructor, ok := autocert.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
	return providerConstructor(cfg.Options)
}

func register(client *lego.Client, cfg *autocert.Config, user *autocert.User) error {
	var reg *registration.Resource
	var err error
	if cfg.EABKid != "" && cfg.EABHmac != "" {
		reg, err = client.Registration.RegisterWithExternalAccountBinding(registration.RegisterEABOptions{
			TermsOfServiceAgreed: true,
			Kid:                  cfg.EABKid,
			HmacEncoded:          cfg.EABHmac,
		})
	} else {
		reg, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	}
	if err != nil {
		return err
	}
	user.Registration = reg
	log.Info().Interface("reg", reg).Msg("acme registered")
	return nil
}

func renewExistingCert(client *lego.Client, cfg *autocert.Config) (*certificate.Resource, error) {
	certPEM, err := os.ReadFile(cfg.CertPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(cfg.KeyPath)
	if err != nil {
		return nil, err
	}
	return client.Certificate.RenewWithOptions(certificate.Resource{
		Certificate: certPEM,
		PrivateKey:  keyPEM,
	}, &certificate.RenewOptions{Bundle: true})
}

func saveCert(cfg *autocert.Config, cert *certificate.Resource) error {
	certDir := filepath.Dir(cfg.CertPath)
	keyDir := filepath.Dir(cfg.KeyPath)
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(keyDir, 0o755); err != nil {
		return err
	}

	certTmp, err := writeTempFile(certDir, "cert-*.pem", cert.Certificate, 0o644)
	if err != nil {
		return err
	}
	defer os.Remove(certTmp)

	keyTmp, err := writeTempFile(keyDir, "key-*.pem", cert.PrivateKey, 0o600)
	if err != nil {
		return err
	}
	defer os.Remove(keyTmp)

	if _, err := tls.X509KeyPair(cert.Certificate, cert.PrivateKey); err != nil {
		return fmt.Errorf("validate certificate and key pair: %w", err)
	}

	if err := os.Rename(keyTmp, cfg.KeyPath); err != nil {
		return err
	}
	if err := os.Rename(certTmp, cfg.CertPath); err != nil {
		return err
	}
	return nil
}

func writeTempFile(dir, pattern string, data []byte, perm os.FileMode) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	closed := false
	defer func() {
		if !closed {
			f.Close()
		}
	}()

	if err := f.Chmod(perm); err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	closed = true
	return f.Name(), nil
}

// ensureHTTPTransportForTLS returns a *http.Transport suitable for mutating TLS settings.
// It accepts nil or *http.Transport; other types are rejected so we never type-assert blindly.
func ensureHTTPTransportForTLS(hc *http.Client) (*http.Transport, error) {
	if hc == nil {
		return nil, fmt.Errorf("HTTP client is nil")
	}
	switch t := hc.Transport.(type) {
	case *http.Transport:
		if t != nil {
			return t.Clone(), nil
		}
	case nil:
		// use default transport clone below
	default:
		return nil, fmt.Errorf("HTTPS client transport must be *http.Transport or nil for custom CA certs, got %T", hc.Transport)
	}
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		return t.Clone(), nil
	}
	return nil, fmt.Errorf("default transport is %T, expected *http.Transport", http.DefaultTransport)
}

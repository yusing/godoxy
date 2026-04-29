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
	"strings"

	"github.com/go-acme/lego/v4/certcrypto"
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
	var privKey *ecdsa.PrivateKey
	var err error

	if cfg.Provider != autocert.ProviderLocal && cfg.Provider != autocert.ProviderPseudo {
		if privKey, err = cfg.LoadACMEKey(); err != nil {
			log.Info().Err(err).Msg("failed to load ACME private key, generating a new one")
			privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				return nil, nil, fmt.Errorf("generate ACME private key: %w", err)
			}
			if err = cfg.SaveACMEKey(privKey); err != nil {
				return nil, nil, fmt.Errorf("save ACME private key: %w", err)
			}
		}
	}

	user := &autocert.User{Email: cfg.Email, Key: privKey}
	legoCfg := lego.NewConfig(user)

	keyType, err := parseCertificateKeyType(cfg.CertificateKeyType)
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
	if len(cfg.CACerts) > 0 {
		certPool, err := lego.CreateCertPool(cfg.CACerts, true)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create cert pool: %w", err)
		}
		rt, err := ensureHTTPTransportForTLS(legoCfg.HTTPClient)
		if err != nil {
			return nil, nil, err
		}
		var tlsCfg *tls.Config
		if rt.TLSClientConfig != nil {
			tlsCfg = rt.TLSClientConfig.Clone()
		} else {
			tlsCfg = &tls.Config{}
		}
		tlsCfg.RootCAs = certPool
		rt.TLSClientConfig = tlsCfg
		legoCfg.HTTPClient.Transport = rt
	}
	return user, legoCfg, nil
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
	if err := os.MkdirAll(filepath.Dir(cfg.CertPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(cfg.KeyPath, cert.PrivateKey, 0o600); err != nil {
		return err
	}
	return os.WriteFile(cfg.CertPath, cert.Certificate, 0o644)
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
	return http.DefaultTransport.(*http.Transport).Clone(), nil
}

func parseCertificateKeyType(s string) (certcrypto.KeyType, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return certcrypto.EC256, nil
	}
	switch strings.ToLower(s) {
	case "ec256", "p256":
		return certcrypto.EC256, nil
	case "ec384", "p384":
		return certcrypto.EC384, nil
	case "rsa2048", "2048":
		return certcrypto.RSA2048, nil
	case "rsa3072", "3072":
		return certcrypto.RSA3072, nil
	case "rsa4096", "4096":
		return certcrypto.RSA4096, nil
	case "rsa8192", "8192":
		return certcrypto.RSA8192, nil
	}
	return "", fmt.Errorf("invalid certificate_key_type %q", s)
}

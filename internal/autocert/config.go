package autocert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"os"
	"regexp"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/lego"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils"
)

type Config struct {
	Email       string         `json:"email,omitempty"`
	Domains     []string       `json:"domains,omitempty"`
	CertPath    string         `json:"cert_path,omitempty"`
	KeyPath     string         `json:"key_path,omitempty"`
	ACMEKeyPath string         `json:"acme_key_path,omitempty"`
	Provider    string         `json:"provider,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
}

var (
	ErrMissingDomain   = gperr.New("missing field 'domains'")
	ErrMissingEmail    = gperr.New("missing field 'email'")
	ErrMissingProvider = gperr.New("missing field 'provider'")
	ErrInvalidDomain   = gperr.New("invalid domain")
	ErrUnknownProvider = gperr.New("unknown provider")
)

const (
	ProviderLocal  = "local"
	ProviderPseudo = "pseudo"
)

var domainOrWildcardRE = regexp.MustCompile(`^\*?([^.]+\.)+[^.]+$`)

// Validate implements the utils.CustomValidator interface.
func (cfg *Config) Validate() gperr.Error {
	if cfg == nil {
		return nil
	}

	if cfg.Provider == "" {
		cfg.Provider = ProviderLocal
		return nil
	}

	b := gperr.NewBuilder("autocert errors")
	if cfg.Provider != ProviderLocal && cfg.Provider != ProviderPseudo {
		if len(cfg.Domains) == 0 {
			b.Add(ErrMissingDomain)
		}
		if cfg.Email == "" {
			b.Add(ErrMissingEmail)
		}
		for i, d := range cfg.Domains {
			if !domainOrWildcardRE.MatchString(d) {
				b.Add(ErrInvalidDomain.Subjectf("domains[%d]", i))
			}
		}
		// check if provider is implemented
		providerConstructor, ok := Providers[cfg.Provider]
		if !ok {
			b.Add(ErrUnknownProvider.
				Subject(cfg.Provider).
				With(gperr.DoYouMean(utils.NearestField(cfg.Provider, Providers))))
		} else {
			_, err := providerConstructor(cfg.Options)
			if err != nil {
				b.Add(err)
			}
		}
	}
	return b.Error()
}

func (cfg *Config) GetLegoConfig() (*User, *lego.Config, gperr.Error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}

	if cfg.CertPath == "" {
		cfg.CertPath = CertFileDefault
	}
	if cfg.KeyPath == "" {
		cfg.KeyPath = KeyFileDefault
	}
	if cfg.ACMEKeyPath == "" {
		cfg.ACMEKeyPath = ACMEKeyFileDefault
	}

	var privKey *ecdsa.PrivateKey
	var err error

	if cfg.Provider != ProviderLocal && cfg.Provider != ProviderPseudo {
		if privKey, err = cfg.LoadACMEKey(); err != nil {
			log.Info().Err(err).Msg("load ACME private key failed")
			log.Info().Msg("generate new ACME private key")
			privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				return nil, nil, gperr.New("generate ACME private key").With(err)
			}
			if err = cfg.SaveACMEKey(privKey); err != nil {
				return nil, nil, gperr.New("save ACME private key").With(err)
			}
		}
	}

	user := &User{
		Email: cfg.Email,
		Key:   privKey,
	}

	legoCfg := lego.NewConfig(user)
	legoCfg.Certificate.KeyType = certcrypto.RSA2048

	return user, legoCfg, nil
}

func (cfg *Config) LoadACMEKey() (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(cfg.ACMEKeyPath)
	if err != nil {
		return nil, err
	}
	return x509.ParseECPrivateKey(data)
}

func (cfg *Config) SaveACMEKey(key *ecdsa.PrivateKey) error {
	data, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.ACMEKeyPath, data, 0o600)
}

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
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type (
	AutocertConfig struct {
		Email       string      `json:"email,omitempty"`
		Domains     []string    `json:"domains,omitempty"`
		CertPath    string      `json:"cert_path,omitempty"`
		KeyPath     string      `json:"key_path,omitempty"`
		ACMEKeyPath string      `json:"acme_key_path,omitempty"`
		Provider    string      `json:"provider,omitempty"`
		Options     ProviderOpt `json:"options,omitempty"`
	}
	ProviderOpt map[string]any
)

var (
	ErrMissingDomain   = gperr.New("missing field 'domains'")
	ErrMissingEmail    = gperr.New("missing field 'email'")
	ErrMissingProvider = gperr.New("missing field 'provider'")
	ErrInvalidDomain   = gperr.New("invalid domain")
	ErrUnknownProvider = gperr.New("unknown provider")
)

var domainOrWildcardRE = regexp.MustCompile(`^\*?([^.]+\.)+[^.]+$`)

// Validate implements the utils.CustomValidator interface.
func (cfg *AutocertConfig) Validate() gperr.Error {
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
		providerConstructor, ok := providersGenMap[cfg.Provider]
		if !ok {
			b.Add(ErrUnknownProvider.
				Subject(cfg.Provider).
				Withf(strutils.DoYouMean(utils.NearestField(cfg.Provider, providersGenMap))))
		} else {
			_, err := providerConstructor(cfg.Options)
			if err != nil {
				b.Add(err)
			}
		}
	}
	return b.Error()
}

func (cfg *AutocertConfig) GetProvider() (*Provider, gperr.Error) {
	if cfg == nil {
		cfg = new(AutocertConfig)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
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
		if privKey, err = cfg.loadACMEKey(); err != nil {
			logging.Info().Err(err).Msg("load ACME private key failed")
			logging.Info().Msg("generate new ACME private key")
			privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				return nil, gperr.New("generate ACME private key").With(err)
			}
			if err = cfg.saveACMEKey(privKey); err != nil {
				return nil, gperr.New("save ACME private key").With(err)
			}
		}
	}

	user := &User{
		Email: cfg.Email,
		key:   privKey,
	}

	legoCfg := lego.NewConfig(user)
	legoCfg.Certificate.KeyType = certcrypto.RSA2048

	return &Provider{
		cfg:     cfg,
		user:    user,
		legoCfg: legoCfg,
	}, nil
}

func (cfg *AutocertConfig) loadACMEKey() (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(cfg.ACMEKeyPath)
	if err != nil {
		return nil, err
	}
	return x509.ParseECPrivateKey(data)
}

func (cfg *AutocertConfig) saveACMEKey(key *ecdsa.PrivateKey) error {
	data, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.ACMEKeyPath, data, 0o600)
}

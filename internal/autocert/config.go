package autocert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"net/http"
	"os"
	"regexp"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	gperr "github.com/yusing/goutils/errs"
)

type Config struct {
	Email       string         `json:"email,omitempty"`
	Domains     []string       `json:"domains,omitempty"`
	CertPath    string         `json:"cert_path,omitempty"`
	KeyPath     string         `json:"key_path,omitempty"`
	ACMEKeyPath string         `json:"acme_key_path,omitempty"`
	Provider    string         `json:"provider,omitempty"`
	Options     map[string]any `json:"options,omitempty"`

	Resolvers []string `json:"resolvers,omitempty"`

	// Custom ACME CA
	CADirURL string   `json:"ca_dir_url,omitempty"`
	CACerts  []string `json:"ca_certs,omitempty"`

	// EAB
	EABKid  string `json:"eab_kid,omitempty" validate:"required_with=EABHmac"`
	EABHmac string `json:"eab_hmac,omitempty" validate:"required_with=EABKid"` // base64 encoded

	HTTPClient *http.Client `json:"-"` // for tests only

	challengeProvider challenge.Provider
}

var (
	ErrMissingDomain   = gperr.New("missing field 'domains'")
	ErrMissingEmail    = gperr.New("missing field 'email'")
	ErrMissingProvider = gperr.New("missing field 'provider'")
	ErrMissingCADirURL = gperr.New("missing field 'ca_dir_url'")
	ErrInvalidDomain   = gperr.New("invalid domain")
	ErrUnknownProvider = gperr.New("unknown provider")
)

const (
	ProviderLocal  = "local"
	ProviderPseudo = "pseudo"
	ProviderCustom = "custom"
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
	if cfg.Provider == ProviderCustom && cfg.CADirURL == "" {
		b.Add(ErrMissingCADirURL)
	}

	if cfg.Provider != ProviderLocal && cfg.Provider != ProviderPseudo {
		if len(cfg.Domains) == 0 {
			b.Add(ErrMissingDomain)
		}
		if cfg.Email == "" {
			b.Add(ErrMissingEmail)
		}
		if cfg.Provider != ProviderCustom {
			for i, d := range cfg.Domains {
				if !domainOrWildcardRE.MatchString(d) {
					b.Add(ErrInvalidDomain.Subjectf("domains[%d]", i))
				}
			}
		}
		// check if provider is implemented
		providerConstructor, ok := Providers[cfg.Provider]
		if !ok {
			if cfg.Provider != ProviderCustom {
				b.Add(ErrUnknownProvider.
					Subject(cfg.Provider).
					With(gperr.DoYouMeanField(cfg.Provider, Providers)))
			}
		} else {
			provider, err := providerConstructor(cfg.Options)
			if err != nil {
				b.Add(err)
			} else {
				cfg.challengeProvider = provider
			}
		}
	}

	if cfg.challengeProvider == nil {
		cfg.challengeProvider, _ = Providers[ProviderLocal](nil)
	}
	return b.Error()
}

func (cfg *Config) dns01Options() []dns01.ChallengeOption {
	return []dns01.ChallengeOption{
		dns01.CondOption(len(cfg.Resolvers) > 0, dns01.AddRecursiveNameservers(cfg.Resolvers)),
	}
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
			log.Info().Err(err).Msg("failed to load ACME private key, generating a now one")
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
	legoCfg.Certificate.KeyType = certcrypto.EC256

	if cfg.HTTPClient != nil {
		legoCfg.HTTPClient = cfg.HTTPClient
	}

	if cfg.CADirURL != "" {
		legoCfg.CADirURL = cfg.CADirURL
	}

	if len(cfg.CACerts) > 0 {
		certPool, err := lego.CreateCertPool(cfg.CACerts, true)
		if err != nil {
			return nil, nil, gperr.New("failed to create cert pool").With(err)
		}
		legoCfg.HTTPClient.Transport.(*http.Transport).TLSClientConfig.RootCAs = certPool
	}

	return user, legoCfg, nil
}

func (cfg *Config) LoadACMEKey() (*ecdsa.PrivateKey, error) {
	if common.IsTest {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(cfg.ACMEKeyPath)
	if err != nil {
		return nil, err
	}
	return x509.ParseECPrivateKey(data)
}

func (cfg *Config) SaveACMEKey(key *ecdsa.PrivateKey) error {
	if common.IsTest {
		return nil
	}
	data, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.ACMEKeyPath, data, 0o600)
}

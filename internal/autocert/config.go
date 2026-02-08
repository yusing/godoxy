package autocert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

type ConfigExtra Config
type Config struct {
	Email       string                       `json:"email,omitempty"`
	Domains     []string                     `json:"domains,omitempty"`
	CertPath    string                       `json:"cert_path,omitempty"`
	KeyPath     string                       `json:"key_path,omitempty"`
	Extra       []ConfigExtra                `json:"extra,omitempty"`
	ACMEKeyPath string                       `json:"acme_key_path,omitempty"` // shared by all extra providers with the same CA directory URL
	Provider    string                       `json:"provider,omitempty"`
	Options     map[string]strutils.Redacted `json:"options,omitempty"`

	Resolvers []string `json:"resolvers,omitempty"`

	// Custom ACME CA
	CADirURL string   `json:"ca_dir_url,omitempty"`
	CACerts  []string `json:"ca_certs,omitempty"`

	// EAB
	EABKid  string `json:"eab_kid,omitempty" validate:"required_with=EABHmac"`
	EABHmac string `json:"eab_hmac,omitempty" validate:"required_with=EABKid"` // base64 encoded

	HTTPClient *http.Client `json:"-"` // for tests only

	challengeProvider challenge.Provider

	idx int // 0: main, 1+: extra[i]
}

var (
	ErrMissingField    = gperr.New("missing field")
	ErrDuplicatedPath  = gperr.New("duplicated path")
	ErrInvalidDomain   = gperr.New("invalid domain")
	ErrUnknownProvider = gperr.New("unknown provider")
)

const (
	ProviderLocal  = "local"
	ProviderPseudo = "pseudo"
	ProviderCustom = "custom"
)

var domainOrWildcardRE = regexp.MustCompile(`^\*?([^.]+\.)+[^.]+$`)

// Validate implements the serialization.CustomValidator interface.
func (cfg *Config) Validate() error {
	seenPaths := make(map[string]int) // path -> provider idx (0 for main, 1+ for extras)
	return cfg.validate(seenPaths)
}

func (cfg *ConfigExtra) Validate() error {
	return nil // done by main config's validate
}

func (cfg *ConfigExtra) AsConfig() *Config {
	return (*Config)(cfg)
}

func (cfg *Config) validate(seenPaths map[string]int) error {
	if cfg.Provider == "" {
		cfg.Provider = ProviderLocal
	}
	if cfg.CertPath == "" {
		cfg.CertPath = CertFileDefault
	}
	if cfg.KeyPath == "" {
		cfg.KeyPath = KeyFileDefault
	}
	if cfg.ACMEKeyPath == "" {
		cfg.ACMEKeyPath = acmeKeyPath(cfg.CADirURL)
	}

	b := gperr.NewBuilder("certificate error")

	// check if cert_path is unique
	if first, ok := seenPaths[cfg.CertPath]; ok {
		b.Add(ErrDuplicatedPath.Subjectf("cert_path %s", cfg.CertPath).Withf("first seen in %s", fmt.Sprintf("extra[%d]", first)))
	} else {
		seenPaths[cfg.CertPath] = cfg.idx
	}

	// check if key_path is unique
	if first, ok := seenPaths[cfg.KeyPath]; ok {
		b.Add(ErrDuplicatedPath.Subjectf("key_path %s", cfg.KeyPath).Withf("first seen in %s", fmt.Sprintf("extra[%d]", first)))
	} else {
		seenPaths[cfg.KeyPath] = cfg.idx
	}

	if cfg.Provider == ProviderCustom && cfg.CADirURL == "" {
		b.Add(ErrMissingField.Subject("ca_dir_url"))
	}

	if cfg.Provider != ProviderLocal && cfg.Provider != ProviderPseudo {
		if len(cfg.Domains) == 0 {
			b.Add(ErrMissingField.Subject("domains"))
		}
		if cfg.Email == "" {
			b.Add(ErrMissingField.Subject("email"))
		}
		if cfg.Provider != ProviderCustom {
			for i, d := range cfg.Domains {
				if !domainOrWildcardRE.MatchString(d) {
					b.Add(ErrInvalidDomain.Subjectf("domains[%d]", i))
				}
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

	if cfg.challengeProvider == nil {
		cfg.challengeProvider, _ = Providers[ProviderLocal](nil)
	}

	if len(cfg.Extra) > 0 {
		for i := range cfg.Extra {
			cfg.Extra[i] = MergeExtraConfig(cfg, &cfg.Extra[i])
			cfg.Extra[i].AsConfig().idx = i + 1
			err := cfg.Extra[i].AsConfig().validate(seenPaths)
			if err != nil {
				b.AddSubjectf(err, "extra[%d]", i)
			}
		}
	}
	return b.Error()
}

func (cfg *Config) dns01Options() []dns01.ChallengeOption {
	return []dns01.ChallengeOption{
		dns01.CondOption(len(cfg.Resolvers) > 0, dns01.AddRecursiveNameservers(cfg.Resolvers)),
	}
}

func (cfg *Config) GetLegoConfig() (*User, *lego.Config, error) {
	var privKey *ecdsa.PrivateKey
	var err error

	if cfg.Provider != ProviderLocal && cfg.Provider != ProviderPseudo {
		if privKey, err = cfg.LoadACMEKey(); err != nil {
			log.Info().Err(err).Msg("failed to load ACME private key, generating a now one")
			privKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				return nil, nil, fmt.Errorf("generate ACME private key: %w", err)
			}
			if err = cfg.SaveACMEKey(privKey); err != nil {
				return nil, nil, fmt.Errorf("save ACME private key: %w", err)
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
			return nil, nil, fmt.Errorf("failed to create cert pool: %w", err)
		}
		legoCfg.HTTPClient.Transport.(*http.Transport).TLSClientConfig.RootCAs = certPool
	}

	return user, legoCfg, nil
}

func MergeExtraConfig(mainCfg *Config, extraCfg *ConfigExtra) ConfigExtra {
	merged := ConfigExtra(*mainCfg)
	merged.Extra = nil
	merged.CertPath = extraCfg.CertPath
	merged.KeyPath = extraCfg.KeyPath
	// NOTE: Using same ACME key as main provider

	if extraCfg.Provider != "" {
		merged.Provider = extraCfg.Provider
	}
	if extraCfg.Email != "" {
		merged.Email = extraCfg.Email
	}
	if len(extraCfg.Domains) > 0 {
		merged.Domains = extraCfg.Domains
	}
	if len(extraCfg.Options) > 0 {
		merged.Options = extraCfg.Options
	}
	if len(extraCfg.Resolvers) > 0 {
		merged.Resolvers = extraCfg.Resolvers
	}
	if extraCfg.CADirURL != "" {
		merged.CADirURL = extraCfg.CADirURL
	}
	if len(extraCfg.CACerts) > 0 {
		merged.CACerts = extraCfg.CACerts
	}
	if extraCfg.EABKid != "" {
		merged.EABKid = extraCfg.EABKid
	}
	if extraCfg.EABHmac != "" {
		merged.EABHmac = extraCfg.EABHmac
	}
	if extraCfg.HTTPClient != nil {
		merged.HTTPClient = extraCfg.HTTPClient
	}
	return merged
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

// acmeKeyPath returns the path to the ACME key file based on the CA directory URL.
// Different CA directory URLs will use different key files to avoid key conflicts.
func acmeKeyPath(caDirURL string) string {
	// Use a hash of the CA directory URL to create a unique key filename
	// Default to "acme" if no custom CA is configured (Let's Encrypt default)
	filename := "acme"
	if caDirURL != "" {
		hash := sha256.Sum256([]byte(caDirURL))
		filename = "acme_" + hex.EncodeToString(hash[:])[:16]
	}
	return filepath.Join(certBasePath, filename+".key")
}

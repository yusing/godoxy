package autocert

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"sync"
	"time"

	autocerttypes "github.com/yusing/godoxy/internal/autocert/types"
)

type staticCertSource struct {
	cert *tls.Certificate
}

func (s staticCertSource) getTLSCert() *tls.Certificate { return s.cert }

type FileCache struct {
	mu    sync.RWMutex
	cfg   *Config
	main  staticCertSource
	extra []*FileCacheEntry
	sni   sniMatcher
}

type FileCacheEntry struct {
	cfg  ConfigExtra
	cert staticCertSource
}

func NewFileCache(cfg *Config) (*FileCache, error) {
	if cfg == nil {
		cfg = new(Config)
		_ = cfg.Validate()
	}
	clone := *cfg
	clone.Extra = append([]ConfigExtra(nil), cfg.Extra...)
	return &FileCache{cfg: &clone}, nil
}

func (c *FileCache) LoadAll() error {
	if c == nil {
		return ErrNoCertificates
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	mainCert, err := loadTLSCert(c.cfg.CertPath, c.cfg.KeyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.main = staticCertSource{}
			c.extra = nil
			c.sni = sniMatcher{}
			return ErrNoCertificates
		}
		return err
	}

	c.main = staticCertSource{cert: mainCert}
	c.extra = c.extra[:0]
	matcher := sniMatcher{}
	matcher.addProvider(c.main)

	for i := range c.cfg.Extra {
		extraCfg := c.cfg.Extra[i]
		cert, err := loadTLSCert(extraCfg.CertPath, extraCfg.KeyPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		entry := &FileCacheEntry{cfg: extraCfg, cert: staticCertSource{cert: cert}}
		c.extra = append(c.extra, entry)
		matcher.addProvider(entry.cert)
	}
	c.sni = matcher
	return nil
}

func (c *FileCache) GetCert(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cert := c.main.getTLSCert()
	if cert == nil {
		return nil, ErrNoCertificates
	}
	if hello == nil || hello.ServerName == "" {
		return cert, nil
	}
	if src := c.sni.match(hello.ServerName); src != nil {
		if matched := src.getTLSCert(); matched != nil {
			return matched, nil
		}
	}
	return cert, nil
}

func (c *FileCache) GetCertInfos() ([]autocerttypes.CertInfo, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	infos := make([]autocerttypes.CertInfo, 0, 1+len(c.extra))
	for _, src := range append([]staticCertSource{c.main}, c.extraSources()...) {
		cert := src.getTLSCert()
		if cert == nil || cert.Leaf == nil {
			continue
		}
		infos = append(infos, autocerttypes.CertInfo{
			Subject:        cert.Leaf.Subject.CommonName,
			Issuer:         cert.Leaf.Issuer.CommonName,
			NotBefore:      cert.Leaf.NotBefore.Unix(),
			NotAfter:       cert.Leaf.NotAfter.Unix(),
			DNSNames:       append([]string(nil), cert.Leaf.DNSNames...),
			EmailAddresses: append([]string(nil), cert.Leaf.EmailAddresses...),
		})
	}
	if len(infos) == 0 {
		return nil, ErrNoCertificates
	}
	return infos, nil
}

func (c *FileCache) ShouldRenewOn() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if cert := c.main.getTLSCert(); cert != nil && cert.Leaf != nil {
		return cert.Leaf.NotAfter.AddDate(0, -1, 0)
	}
	return time.Time{}
}

func (c *FileCache) extraSources() []staticCertSource {
	out := make([]staticCertSource, 0, len(c.extra))
	for _, entry := range c.extra {
		out = append(out, entry.cert)
	}
	return out
}

func loadTLSCert(certPath, keyPath string) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	if len(cert.Certificate) == 0 {
		return nil, ErrNoCertificates
	}
	if leaf, err := x509.ParseCertificate(cert.Certificate[0]); err == nil {
		cert.Leaf = leaf
	}
	return &cert, nil
}

var _ certSource = staticCertSource{}

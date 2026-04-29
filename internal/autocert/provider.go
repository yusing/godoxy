package autocert

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/notif"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
)

type (
	Provider struct {
		mu     sync.RWMutex
		obtain sync.Mutex

		logger zerolog.Logger

		cfg         *Config
		lastFailure time.Time

		lastFailureFile string

		tlsCert      *tls.Certificate
		certExpiries CertExpiries

		extraProviders []*Provider
		sniMatcher     sniMatcher

		forceRenewalCh     chan struct{}
		forceRenewalDoneCh atomic.Pointer[chan struct{}]

		scheduleRenewalOnce sync.Once
	}

	CertExpiries map[string]time.Time

	RenewMode uint8
)

var ErrNoCertificates = errors.New("no certificates found")

const (
	// renew failed for whatever reason, 1 hour cooldown
	renewalCooldownDuration = 1 * time.Hour
	// prevents cert request docker compose across restarts with `restart: always` (non-zero exit code)
	requestCooldownDuration = 15 * time.Second
)

const (
	renewModeForce = iota
	renewModeIfNeeded
)

func NewProvider(cfg *Config) (*Provider, error) {
	p := &Provider{
		cfg:             cfg,
		lastFailureFile: lastFailureFileFor(cfg.CertPath, cfg.KeyPath),
		forceRenewalCh:  make(chan struct{}, 1),
	}
	p.forceRenewalDoneCh.Store(&emptyForceRenewalDoneCh)

	if cfg.idx == 0 {
		p.logger = log.With().Str("provider", "main").Logger()
	} else {
		p.logger = log.With().Str("provider", fmt.Sprintf("extra[%d]", cfg.idx)).Logger()
	}
	if err := p.setupExtraProviders(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Provider) GetCert(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	tlsCert := p.getTLSCert()
	if tlsCert == nil {
		return nil, ErrNoCertificates
	}
	if hello == nil || hello.ServerName == "" {
		return tlsCert, nil
	}
	if prov := p.getSNIMatcher().match(hello.ServerName); prov != nil {
		if cert := prov.getTLSCert(); cert != nil {
			return cert, nil
		}
	}
	return tlsCert, nil
}

func (p *Provider) GetCertInfos() ([]autocert.CertInfo, error) {
	allProviders := p.allProviders()
	certInfos := make([]autocert.CertInfo, 0, len(allProviders))
	for _, provider := range allProviders {
		tlsCert := provider.getTLSCert()
		if tlsCert == nil || tlsCert.Leaf == nil {
			continue
		}
		certInfos = append(certInfos, autocert.CertInfo{
			Subject:        tlsCert.Leaf.Subject.CommonName,
			Issuer:         tlsCert.Leaf.Issuer.CommonName,
			NotBefore:      tlsCert.Leaf.NotBefore.Unix(),
			NotAfter:       tlsCert.Leaf.NotAfter.Unix(),
			DNSNames:       tlsCert.Leaf.DNSNames,
			EmailAddresses: tlsCert.Leaf.EmailAddresses,
		})
	}

	if len(certInfos) == 0 {
		return nil, ErrNoCertificates
	}
	return certInfos, nil
}

func (p *Provider) GetName() string {
	if p.cfg.idx == 0 {
		return "main"
	}
	return fmt.Sprintf("extra[%d]", p.cfg.idx)
}

func (p *Provider) fmtError(err error) error {
	return gperr.PrependSubject(err, "provider: "+p.GetName())
}

func (p *Provider) GetCertPath() string {
	return p.cfg.CertPath
}

func (p *Provider) GetKeyPath() string {
	return p.cfg.KeyPath
}

func (p *Provider) GetExpiries() CertExpiries {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return maps.Clone(p.certExpiries)
}

func (p *Provider) GetLastFailure() (time.Time, error) {
	if common.IsTest {
		return time.Time{}, nil
	}

	p.mu.RLock()
	lastFailure := p.lastFailure
	p.mu.RUnlock()

	if lastFailure.IsZero() {
		data, err := os.ReadFile(p.lastFailureFile)
		if err != nil {
			if !os.IsNotExist(err) {
				return time.Time{}, err
			}
		} else {
			parsed, _ := time.Parse(time.RFC3339, string(data))
			p.mu.Lock()
			p.lastFailure = parsed
			lastFailure = p.lastFailure
			p.mu.Unlock()
		}
	}
	return lastFailure, nil
}

func (p *Provider) UpdateLastFailure() error {
	if common.IsTest {
		return nil
	}
	t := time.Now()
	p.mu.Lock()
	p.lastFailure = t
	p.mu.Unlock()
	return os.WriteFile(p.lastFailureFile, t.AppendFormat(nil, time.RFC3339), 0o600)
}

func (p *Provider) ClearLastFailure() error {
	if common.IsTest {
		return nil
	}
	p.mu.Lock()
	p.lastFailure = time.Time{}
	p.mu.Unlock()
	err := os.Remove(p.lastFailureFile)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// allProviders returns all providers including this provider and all extra providers.
func (p *Provider) allProviders() []*Provider {
	return append([]*Provider{p}, p.extraProviders...)
}

// ObtainCertIfNotExistsAll obtains a new certificate for this provider and all extra providers if they do not exist.
func (p *Provider) ObtainCertIfNotExistsAll(ctx context.Context) error {
	errs := gperr.NewGroup("obtain cert error")

	for _, provider := range p.allProviders() {
		errs.Go(func() error {
			if err := provider.obtainCertIfNotExists(ctx); err != nil {
				return gperr.PrependSubject(err, provider.GetName())
			}
			return nil
		})
	}

	err := errs.Wait().Error()
	p.rebuildSNIMatcher()
	return err
}

// obtainCertIfNotExists obtains a new certificate for this provider if it does not exist.
func (p *Provider) obtainCertIfNotExists(ctx context.Context) error {
	err := p.loadCert()
	if err == nil {
		return nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// check last failure
	lastFailure, err := p.GetLastFailure()
	if err != nil {
		return fmt.Errorf("failed to get last failure: %w", err)
	}
	if !lastFailure.IsZero() && time.Since(lastFailure) < requestCooldownDuration {
		return fmt.Errorf("still in cooldown until %s", strutils.FormatTime(lastFailure.Add(requestCooldownDuration).Local()))
	}

	p.logger.Info().Msg("cert not found, obtaining new cert")
	return p.ObtainCert(ctx)
}

// ObtainCertAll renews existing certificates or obtains new certificates for this provider and all extra providers.
func (p *Provider) ObtainCertAll(ctx context.Context) error {
	errs := gperr.NewGroup("obtain cert error")
	for _, provider := range p.allProviders() {
		prov := provider
		errs.Go(func() error {
			if err := prov.ObtainCert(ctx); err != nil {
				return gperr.PrependSubject(err, prov.GetName())
			}
			return nil
		})
	}

	err := errs.Wait().Error()
	p.rebuildSNIMatcher()
	return err
}

// ObtainCert renews existing certificate or obtains a new certificate for this provider.
func (p *Provider) ObtainCert(ctx context.Context) error {
	p.obtain.Lock()
	defer p.obtain.Unlock()

	if !shouldUseLocalAutocertOperations(p.cfg.Provider) {
		if err := obtainCertUsingBinary(ctx, p.cfg.CertPath); err != nil {
			return err
		}
		if err := p.loadCert(); err != nil {
			return err
		}
		p.rebuildSNIMatcher()
		return nil
	}

	return p.obtainCertLocally(ctx)
}

func (p *Provider) LoadCertAll() error {
	var errs gperr.Builder
	for _, provider := range p.allProviders() {
		if err := provider.loadCert(); err != nil {
			errs.Add(provider.fmtError(err))
		}
	}
	p.rebuildSNIMatcher()
	return errs.Error()
}

func (p *Provider) loadCert() error {
	cert, err := tls.LoadX509KeyPair(p.cfg.CertPath, p.cfg.KeyPath)
	if err != nil {
		return err
	}

	expiries, err := getCertExpiries(&cert)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.tlsCert = &cert
	p.certExpiries = expiries
	p.mu.Unlock()

	return nil
}

// PrintCertExpiriesAll prints the certificate expiries for this provider and all extra providers.
func (p *Provider) PrintCertExpiriesAll() {
	for _, provider := range p.allProviders() {
		for domain, expiry := range provider.GetExpiries() {
			p.logger.Info().Str("domain", domain).Msgf("certificate expire on %s", strutils.FormatTime(expiry))
		}
	}
}

// ShouldRenewOn returns the time at which the certificate should be renewed.
func (p *Provider) ShouldRenewOn() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, expiry := range p.certExpiries {
		return expiry.AddDate(0, -1, 0) // 1 month before
	}
	// this line should never be reached in production, but will be useful for testing
	return time.Now().AddDate(0, 1, 0) // 1 month after
}

// ForceExpiryAll triggers immediate certificate renewal for this provider and all extra providers.
// Returns true if the renewal was triggered, false if the renewal was dropped.
//
// If at least one renewal is triggered, returns true.
func (p *Provider) ForceExpiryAll() (ok bool) {
	doneCh := p.beginForceRenewal()
	if doneCh != nil {
		select {
		case p.forceRenewalCh <- struct{}{}:
			ok = true
		default:
			p.finishForceRenewal(doneCh)
		}
	}

	for _, ep := range p.extraProviders {
		if ep.ForceExpiryAll() {
			ok = true
		}
	}

	return ok
}

// WaitRenewalDone waits for the renewal to complete.
// Returns false if the renewal was dropped.
func (p *Provider) WaitRenewalDone(ctx context.Context) bool {
	done := p.forceRenewalDoneCh.Load()
	if done == nil || *done == nil {
		return false
	}
	select {
	case <-*done:
	case <-ctx.Done():
		return false
	}

	for _, ep := range p.extraProviders {
		if !ep.WaitRenewalDone(ctx) {
			return false
		}
	}
	return true
}

func (p *Provider) beginForceRenewal() *chan struct{} {
	for {
		done := p.forceRenewalDoneCh.Load()
		switch {
		case done == nil:
			return nil
		case *done == nil:
			next := make(chan struct{})
			if p.forceRenewalDoneCh.CompareAndSwap(done, &next) {
				return &next
			}
		default:
			select {
			case <-*done:
				next := make(chan struct{})
				if p.forceRenewalDoneCh.CompareAndSwap(done, &next) {
					return &next
				}
			default:
				return nil
			}
		}
	}
}

func (p *Provider) finishForceRenewal(done *chan struct{}) {
	if done == nil || *done == nil {
		return
	}
	select {
	case <-*done:
	default:
		close(*done)
	}
}

// ScheduleRenewalAll schedules the renewal of the certificate for this provider and all extra providers.
func (p *Provider) ScheduleRenewalAll(parent task.Parent) {
	p.scheduleRenewalOnce.Do(func() {
		p.scheduleRenewal(parent)
	})
	for _, ep := range p.extraProviders {
		ep.scheduleRenewalOnce.Do(func() {
			ep.scheduleRenewal(parent)
		})
	}
}

var emptyForceRenewalDoneCh chan struct{}

// scheduleRenewal schedules the renewal of the certificate for this provider.
func (p *Provider) scheduleRenewal(parent task.Parent) {
	if p.cfg.Provider == ProviderLocal || p.cfg.Provider == ProviderPseudo {
		return
	}

	timer := time.NewTimer(time.Until(p.ShouldRenewOn()))
	task := parent.Subtask("cert-renew-scheduler:"+filepath.Base(p.cfg.CertPath), true)

	renew := func(renewMode RenewMode) {
		defer func() {
			p.finishForceRenewal(p.forceRenewalDoneCh.Load())
		}()

		ctx, cancel := context.WithTimeout(task.Context(), 5*time.Minute)
		defer cancel()

		renewed, err := p.renew(ctx, renewMode)
		if err != nil {
			log.Warn().Err(p.fmtError(err)).Msg("autocert: cert renew failed")
			notif.Notify(&notif.LogMessage{
				Level: zerolog.ErrorLevel,
				Title: "SSL certificate renewal failed for " + p.GetName(),
				Body:  notif.MessageBody(err.Error()),
			})
			return
		}
		if renewed {
			p.rebuildSNIMatcher()

			notif.Notify(&notif.LogMessage{
				Level: zerolog.InfoLevel,
				Title: "SSL certificate renewed for " + p.GetName(),
				Body:  notif.ListBody(p.cfg.Domains),
			})

			// Reset on success
			if err := p.ClearLastFailure(); err != nil {
				log.Warn().Err(p.fmtError(err)).Msg("autocert: failed to clear last failure")
			}
			timer.Reset(time.Until(p.ShouldRenewOn()))
		}
	}

	go func() {
		defer timer.Stop()
		defer task.Finish(nil)

		for {
			select {
			case <-task.Context().Done():
				return
			case <-p.forceRenewalCh:
				renew(renewModeForce)
			case <-timer.C:
				renew(renewModeIfNeeded)
			}
		}
	}()
}

func getCertExpiries(cert *tls.Certificate) (CertExpiries, error) {
	r := make(CertExpiries, len(cert.Certificate))
	for _, cert := range cert.Certificate {
		x509Cert, err := x509.ParseCertificate(cert)
		if err != nil {
			return nil, err
		}
		if x509Cert.IsCA {
			continue
		}
		r[x509Cert.Subject.CommonName] = x509Cert.NotAfter
		for i := range x509Cert.DNSNames {
			r[x509Cert.DNSNames[i]] = x509Cert.NotAfter
		}
	}
	return r, nil
}

func lastFailureFileFor(certPath, keyPath string) string {
	dir := filepath.Dir(certPath)
	sum := sha256.Sum256([]byte(certPath + "|" + keyPath))
	return filepath.Join(dir, fmt.Sprintf(".last_failure-%x", sum[:6]))
}

func (p *Provider) rebuildSNIMatcher() {
	if p.cfg.idx != 0 { // only main provider has extra providers
		return
	}

	matcher := sniMatcher{}
	matcher.addProvider(p)
	for _, ep := range p.extraProviders {
		matcher.addProvider(ep)
	}

	p.mu.Lock()
	p.sniMatcher = matcher
	p.mu.Unlock()
}

func (p *Provider) getSNIMatcher() *sniMatcher {
	p.mu.RLock()
	defer p.mu.RUnlock()
	matcher := p.sniMatcher
	return &matcher
}

func (p *Provider) getTLSCert() *tls.Certificate {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tlsCert
}

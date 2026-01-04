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
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/notif"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
)

type (
	Provider struct {
		logger zerolog.Logger

		cfg         *Config
		user        *User
		legoCfg     *lego.Config
		client      *lego.Client
		lastFailure time.Time

		lastFailureFile string

		legoCert     *certificate.Resource
		tlsCert      *tls.Certificate
		certExpiries CertExpiries

		extraProviders []*Provider
		sniMatcher     sniMatcher

		forceRenewalCh     chan struct{}
		forceRenewalDoneCh atomic.Value // chan struct{}

		scheduleRenewalOnce sync.Once
	}

	CertExpiries map[string]time.Time
	RenewMode    uint8
)

var ErrNoCertificate = errors.New("no certificate found")

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

// could be nil
var ActiveProvider atomic.Pointer[Provider]

func NewProvider(cfg *Config, user *User, legoCfg *lego.Config) (*Provider, error) {
	p := &Provider{
		cfg:             cfg,
		user:            user,
		legoCfg:         legoCfg,
		lastFailureFile: lastFailureFileFor(cfg.CertPath, cfg.KeyPath),
		forceRenewalCh:  make(chan struct{}, 1),
	}
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
	if p.tlsCert == nil {
		return nil, ErrNoCertificate
	}
	if hello == nil || hello.ServerName == "" {
		return p.tlsCert, nil
	}
	if prov := p.sniMatcher.match(hello.ServerName); prov != nil && prov.tlsCert != nil {
		return prov.tlsCert, nil
	}
	return p.tlsCert, nil
}

func (p *Provider) GetName() string {
	if p.cfg.idx == 0 {
		return "main"
	}
	return fmt.Sprintf("extra[%d]", p.cfg.idx)
}

func (p *Provider) fmtError(err error) error {
	return gperr.PrependSubject(fmt.Sprintf("provider: %s", p.GetName()), err)
}

func (p *Provider) GetCertPath() string {
	return p.cfg.CertPath
}

func (p *Provider) GetKeyPath() string {
	return p.cfg.KeyPath
}

func (p *Provider) GetExpiries() CertExpiries {
	return p.certExpiries
}

func (p *Provider) GetLastFailure() (time.Time, error) {
	if common.IsTest {
		return time.Time{}, nil
	}

	if p.lastFailure.IsZero() {
		data, err := os.ReadFile(p.lastFailureFile)
		if err != nil {
			if !os.IsNotExist(err) {
				return time.Time{}, err
			}
		} else {
			p.lastFailure, _ = time.Parse(time.RFC3339, string(data))
		}
	}
	return p.lastFailure, nil
}

func (p *Provider) UpdateLastFailure() error {
	if common.IsTest {
		return nil
	}
	t := time.Now()
	p.lastFailure = t
	return os.WriteFile(p.lastFailureFile, t.AppendFormat(nil, time.RFC3339), 0o600)
}

func (p *Provider) ClearLastFailure() error {
	if common.IsTest {
		return nil
	}
	p.lastFailure = time.Time{}
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
func (p *Provider) ObtainCertIfNotExistsAll() error {
	errs := gperr.NewGroup("obtain cert error")

	for _, provider := range p.allProviders() {
		errs.Go(func() error {
			if err := provider.obtainCertIfNotExists(); err != nil {
				return fmt.Errorf("failed to obtain cert for %s: %w", provider.GetName(), err)
			}
			return nil
		})
	}

	p.rebuildSNIMatcher()
	return errs.Wait().Error()
}

// obtainCertIfNotExists obtains a new certificate for this provider if it does not exist.
func (p *Provider) obtainCertIfNotExists() error {
	err := p.LoadCert()
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
	return p.ObtainCert()
}

// ObtainCertAll renews existing certificates or obtains new certificates for this provider and all extra providers.
func (p *Provider) ObtainCertAll() error {
	errs := gperr.NewGroup("obtain cert error")
	for _, provider := range p.allProviders() {
		errs.Go(func() error {
			if err := provider.obtainCertIfNotExists(); err != nil {
				return fmt.Errorf("failed to obtain cert for %s: %w", provider.GetName(), err)
			}
			return nil
		})
	}
	return errs.Wait().Error()
}

// ObtainCert renews existing certificate or obtains a new certificate for this provider.
func (p *Provider) ObtainCert() error {
	if p.cfg.Provider == ProviderLocal {
		return nil
	}

	if p.cfg.Provider == ProviderPseudo {
		p.logger.Info().Msg("init client for pseudo provider")
		<-time.After(time.Second)
		p.logger.Info().Msg("registering acme for pseudo provider")
		<-time.After(time.Second)
		p.logger.Info().Msg("obtained cert for pseudo provider")
		return nil
	}

	if p.client == nil {
		if err := p.initClient(); err != nil {
			return err
		}
	}

	// mark it as failed first, clear it later if successful
	// in case the process crashed / failed to renew, we put it on a cooldown
	// this prevents rate limiting by the ACME server
	if err := p.UpdateLastFailure(); err != nil {
		return fmt.Errorf("failed to update last failure: %w", err)
	}

	if p.user.Registration == nil {
		if err := p.registerACME(); err != nil {
			return err
		}
	}

	var cert *certificate.Resource
	var err error

	if p.legoCert != nil {
		cert, err = p.client.Certificate.RenewWithOptions(*p.legoCert, &certificate.RenewOptions{
			Bundle: true,
		})
		if err != nil {
			p.legoCert = nil
			log.Err(err).Msg("cert renew failed, fallback to obtain")
		} else {
			p.legoCert = cert
		}
	}

	if cert == nil {
		cert, err = p.client.Certificate.Obtain(certificate.ObtainRequest{
			Domains: p.cfg.Domains,
			Bundle:  true,
		})
		if err != nil {
			return err
		}
	}

	if err = p.saveCert(cert); err != nil {
		return err
	}

	tlsCert, err := tls.X509KeyPair(cert.Certificate, cert.PrivateKey)
	if err != nil {
		return err
	}

	expiries, err := getCertExpiries(&tlsCert)
	if err != nil {
		return err
	}
	p.tlsCert = &tlsCert
	p.certExpiries = expiries
	p.rebuildSNIMatcher()

	if err := p.ClearLastFailure(); err != nil {
		return fmt.Errorf("failed to clear last failure: %w", err)
	}
	return nil
}

func (p *Provider) LoadCert() error {
	var errs gperr.Builder
	cert, err := tls.LoadX509KeyPair(p.cfg.CertPath, p.cfg.KeyPath)
	if err != nil {
		errs.Addf("load SSL certificate: %w", p.fmtError(err))
	}

	expiries, err := getCertExpiries(&cert)
	if err != nil {
		errs.Addf("parse SSL certificate: %w", p.fmtError(err))
	}

	p.tlsCert = &cert
	p.certExpiries = expiries

	for _, ep := range p.extraProviders {
		if err := ep.LoadCert(); err != nil {
			errs.Add(err)
		}
	}

	p.rebuildSNIMatcher()
	return errs.Error()
}

// PrintCertExpiriesAll prints the certificate expiries for this provider and all extra providers.
func (p *Provider) PrintCertExpiriesAll() {
	for _, provider := range p.allProviders() {
		for domain, expiry := range provider.certExpiries {
			p.logger.Info().Str("domain", domain).Msgf("certificate expire on %s", strutils.FormatTime(expiry))
		}
	}
}

// ShouldRenewOn returns the time at which the certificate should be renewed.
func (p *Provider) ShouldRenewOn() time.Time {
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
	doneCh := make(chan struct{})
	if swapped := p.forceRenewalDoneCh.CompareAndSwap(nil, doneCh); !swapped { // already in progress
		close(doneCh)
		return false
	}

	select {
	case p.forceRenewalCh <- struct{}{}:
		ok = true
	default:
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
	done, ok := p.forceRenewalDoneCh.Load().(chan struct{})
	if !ok || done == nil {
		return false
	}
	select {
	case <-done:
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

var emptyForceRenewalDoneCh any = chan struct{}(nil)

// scheduleRenewal schedules the renewal of the certificate for this provider.
func (p *Provider) scheduleRenewal(parent task.Parent) {
	if p.GetName() == ProviderLocal || p.GetName() == ProviderPseudo {
		return
	}

	timer := time.NewTimer(time.Until(p.ShouldRenewOn()))
	task := parent.Subtask("cert-renew-scheduler:"+filepath.Base(p.cfg.CertPath), true)

	renew := func(renewMode RenewMode) {
		defer func() {
			if done, ok := p.forceRenewalDoneCh.Swap(emptyForceRenewalDoneCh).(chan struct{}); ok && done != nil {
				close(done)
			}
		}()

		renewed, err := p.renew(renewMode)
		if err != nil {
			gperr.LogWarn("autocert: cert renew failed", p.fmtError(err))
			notif.Notify(&notif.LogMessage{
				Level: zerolog.ErrorLevel,
				Title: fmt.Sprintf("SSL certificate renewal failed for %s", p.GetName()),
				Body:  notif.MessageBody(err.Error()),
			})
			return
		}
		if renewed {
			p.rebuildSNIMatcher()

			notif.Notify(&notif.LogMessage{
				Level: zerolog.InfoLevel,
				Title: fmt.Sprintf("SSL certificate renewed for %s", p.GetName()),
				Body:  notif.ListBody(p.cfg.Domains),
			})

			// Reset on success
			if err := p.ClearLastFailure(); err != nil {
				gperr.LogWarn("autocert: failed to clear last failure", p.fmtError(err))
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

func (p *Provider) initClient() error {
	legoClient, err := lego.NewClient(p.legoCfg)
	if err != nil {
		return err
	}

	err = legoClient.Challenge.SetDNS01Provider(p.cfg.challengeProvider, p.cfg.dns01Options()...)
	if err != nil {
		return err
	}

	p.client = legoClient
	return nil
}

func (p *Provider) registerACME() error {
	if p.user.Registration != nil {
		return nil
	}

	reg, err := p.client.Registration.ResolveAccountByKey()
	if err == nil {
		p.user.Registration = reg
		log.Info().Msg("reused acme registration from private key")
		return nil
	}

	if p.cfg.EABKid != "" && p.cfg.EABHmac != "" {
		reg, err = p.client.Registration.RegisterWithExternalAccountBinding(registration.RegisterEABOptions{
			TermsOfServiceAgreed: true,
			Kid:                  p.cfg.EABKid,
			HmacEncoded:          p.cfg.EABHmac,
		})
	} else {
		reg, err = p.client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	}
	if err != nil {
		return err
	}
	p.user.Registration = reg
	log.Info().Interface("reg", reg).Msg("acme registered")
	return nil
}

func (p *Provider) saveCert(cert *certificate.Resource) error {
	if common.IsTest {
		return nil
	}
	/* This should have been done in setup
	but double check is always a good choice.*/
	_, err := os.Stat(filepath.Dir(p.cfg.CertPath))
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(filepath.Dir(p.cfg.CertPath), 0o755); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	err = os.WriteFile(p.cfg.KeyPath, cert.PrivateKey, 0o600) // -rw-------
	if err != nil {
		return err
	}

	err = os.WriteFile(p.cfg.CertPath, cert.Certificate, 0o644) // -rw-r--r--
	if err != nil {
		return err
	}
	return nil
}

func (p *Provider) certState() CertState {
	if time.Now().After(p.ShouldRenewOn()) {
		return CertStateExpired
	}

	if len(p.certExpiries) != len(p.cfg.Domains) {
		return CertStateMismatch
	}

	for i := range len(p.cfg.Domains) {
		if _, ok := p.certExpiries[p.cfg.Domains[i]]; !ok {
			log.Info().Msgf("autocert domains mismatch: cert: %s, wanted: %s",
				strings.Join(slices.Collect(maps.Keys(p.certExpiries)), ", "),
				strings.Join(p.cfg.Domains, ", "))
			return CertStateMismatch
		}
	}

	return CertStateValid
}

func (p *Provider) renew(mode RenewMode) (renewed bool, err error) {
	if p.cfg.Provider == ProviderLocal {
		return false, nil
	}

	if mode != renewModeForce {
		// Retry after 1 hour on failure
		lastFailure, err := p.GetLastFailure()
		if err != nil {
			return false, fmt.Errorf("failed to get last failure: %w", err)
		}
		if !lastFailure.IsZero() && time.Since(lastFailure) < renewalCooldownDuration {
			until := lastFailure.Add(renewalCooldownDuration).Local()
			return false, fmt.Errorf("still in cooldown until %s", strutils.FormatTime(until))
		}
	}

	if mode == renewModeIfNeeded {
		switch p.certState() {
		case CertStateExpired:
			log.Info().Msg("certs expired, renewing")
		case CertStateMismatch:
			log.Info().Msg("cert domains mismatch with config, renewing")
		default:
			return false, nil
		}
	}

	if mode == renewModeForce {
		log.Info().Msg("force renewing cert by user request")
	}

	if err := p.ObtainCert(); err != nil {
		return false, err
	}
	return true, nil
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

	p.sniMatcher = sniMatcher{}
	p.sniMatcher.addProvider(p)
	for _, ep := range p.extraProviders {
		p.sniMatcher.addProvider(ep)
	}
}

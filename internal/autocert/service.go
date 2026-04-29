package autocert

import (
	"context"
	"crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	autocerttypes "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/goutils/task"
)

type runOnce interface {
	Run(ctx context.Context, mode RunMode, cfgPath string) error
}

type runnerFunc func(ctx context.Context, mode RunMode, cfgPath string) error

func (f runnerFunc) Run(ctx context.Context, mode RunMode, cfgPath string) error {
	return f(ctx, mode, cfgPath)
}

type Service struct {
	cache        *FileCache
	runner       runOnce
	cfg          *Config
	snapshotPath string

	schedulerOnce sync.Once

	mu      sync.Mutex
	runDone chan struct{}
	runErr  error
	running atomic.Bool
}

func NewService(parent task.Parent, cfg *Config, helperBin string) (*Service, error) {
	if cfg == nil {
		cfg = new(Config)
	}
	clone := *cfg
	clone.Extra = append([]ConfigExtra(nil), cfg.Extra...)
	if err := clone.Validate(); err != nil {
		return nil, err
	}

	cache, err := NewFileCache(&clone)
	if err != nil {
		return nil, err
	}
	if helperBin == "" {
		helperBin = DefaultHelperBinary()
	}
	svc := &Service{
		cache:        cache,
		runner:       newRunner(helperBin),
		cfg:          &clone,
		snapshotPath: RuntimeConfigPath(),
	}
	if err := svc.writeSnapshot(); err != nil {
		return nil, err
	}

	if svc.requiresHelper() {
		if err := svc.runExclusive(parentContext(parent), RunModeObtainIfMissing); err != nil {
			return nil, err
		}
	} else if err := svc.cache.LoadAll(); err != nil && !errors.Is(err, ErrNoCertificates) {
		return nil, err
	}

	svc.PrintCertExpiriesAll()
	if parent != nil {
		svc.ScheduleRenewalAll(parent)
	}
	return svc, nil
}

func parentContext(parent task.Parent) context.Context {
	if parent == nil {
		return context.Background()
	}
	return parent.Context()
}

func (s *Service) requiresHelper() bool {
	for _, cfg := range append([]*Config{s.cfg}, s.extraConfigs()...) {
		if cfg.Provider != ProviderLocal && cfg.Provider != ProviderPseudo {
			return true
		}
	}
	return false
}

func (s *Service) extraConfigs() []*Config {
	out := make([]*Config, 0, len(s.cfg.Extra))
	for i := range s.cfg.Extra {
		out = append(out, s.cfg.Extra[i].AsConfig())
	}
	return out
}

func (s *Service) writeSnapshot() error {
	if err := os.MkdirAll(filepath.Dir(s.snapshotPath), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(s.cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(s.snapshotPath, data, 0o600)
}

func (s *Service) GetCert(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return s.cache.GetCert(hello)
}

func (s *Service) GetCertInfos() ([]autocerttypes.CertInfo, error) {
	return s.cache.GetCertInfos()
}

func (s *Service) ScheduleRenewalAll(parent task.Parent) {
	s.schedulerOnce.Do(func() {
		if parent == nil || !s.requiresHelper() {
			return
		}
		t := parent.Subtask("autocert-renew-scheduler", true)
		go s.scheduleLoop(t)
	})
}

func (s *Service) ObtainCertAll() error {
	return s.runExclusive(context.Background(), RunModeRenewAll)
}

func (s *Service) ForceExpiryAll() bool {
	done := make(chan struct{})
	if !s.beginRun(done) {
		return false
	}
	go func() {
		err := s.runOnce(context.Background(), RunModeRenewAll)
		s.finishRun(done, err)
	}()
	return true
}

func (s *Service) WaitRenewalDone(ctx context.Context) bool {
	s.mu.Lock()
	done := s.runDone
	s.mu.Unlock()
	if done == nil {
		return false
	}
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Service) PrintCertExpiriesAll() {
	infos, err := s.cache.GetCertInfos()
	if err != nil {
		return
	}
	for _, info := range infos {
		log.Info().
			Str("subject", info.Subject).
			Time("not_after", time.Unix(info.NotAfter, 0)).
			Msg("certificate expiry")
	}
}

func (s *Service) runExclusive(ctx context.Context, mode RunMode) error {
	done := make(chan struct{})
	if !s.beginRun(done) {
		return ErrRunnerBusy
	}
	err := s.runOnce(ctx, mode)
	s.finishRun(done, err)
	return err
}

func (s *Service) runOnce(ctx context.Context, mode RunMode) error {
	if err := s.writeSnapshot(); err != nil {
		return err
	}
	if s.requiresHelper() {
		if err := s.runner.Run(ctx, mode, s.snapshotPath); err != nil {
			return err
		}
	}
	if err := s.cache.LoadAll(); err != nil {
		if errors.Is(err, ErrNoCertificates) && !s.requiresHelper() {
			return nil
		}
		return err
	}
	return nil
}

func (s *Service) beginRun(done chan struct{}) bool {
	if !s.running.CompareAndSwap(false, true) {
		return false
	}
	s.mu.Lock()
	s.runDone = done
	s.runErr = nil
	s.mu.Unlock()
	return true
}

func (s *Service) finishRun(done chan struct{}, err error) {
	if err != nil {
		log.Warn().Err(err).Msg("autocert helper failed")
		notif.Notify(&notif.LogMessage{
			Level: zerolog.ErrorLevel,
			Title: "SSL certificate renewal failed",
			Body:  notif.MessageBody(err.Error()),
		})
	}
	s.mu.Lock()
	s.runErr = err
	close(done)
	s.mu.Unlock()
	s.running.Store(false)
}

func (s *Service) scheduleLoop(t *task.Task) {
	defer t.Finish(nil)

	for {
		timer := time.NewTimer(s.nextRenewDelay())
		select {
		case <-t.Context().Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		err := s.runExclusive(t.Context(), RunModeRenewAll)
		if err == nil {
			notif.Notify(&notif.LogMessage{
				Level: zerolog.InfoLevel,
				Title: "SSL certificate renewed",
				Body:  notif.ListBody(s.cfg.Domains),
			})
			if s.nextRenewDelay() == 0 && !sleepOrDone(t.Context(), renewalCooldownDuration) {
				return
			}
			continue
		}
		if !errors.Is(err, ErrRunnerBusy) {
			log.Warn().Err(err).Msg("autocert scheduled renewal failed")
		}
		if !sleepOrDone(t.Context(), renewalCooldownDuration) {
			return
		}
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Service) nextRenewDelay() time.Duration {
	infos, err := s.cache.GetCertInfos()
	if err != nil || len(infos) == 0 {
		return time.Minute
	}

	var next time.Time
	for _, info := range infos {
		renewAt := time.Unix(info.NotAfter, 0).AddDate(0, -1, 0)
		if next.IsZero() || renewAt.Before(next) {
			next = renewAt
		}
	}
	delay := time.Until(next)
	if delay < 0 {
		return 0
	}
	return delay
}

var _ autocerttypes.Provider = (*Service)(nil)

var newRunner = func(bin string) runOnce {
	return NewRunner(bin)
}

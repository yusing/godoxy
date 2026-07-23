package config

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/metrics/systeminfo"
	"github.com/yusing/godoxy/internal/metrics/uptime"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/routing"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/events"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
)

type managedState interface {
	configtypes.State
	PreparationIssues() []configtypes.ActivationIssue
	discardTmpLog()
	setStatus(configtypes.RuntimeStatus)
	setActivation(configtypes.ActivationReport, configtypes.ActivationHealth)
}

type runtimeFactory func() managedState

type metricsService interface {
	Activate() configtypes.ComponentActivation
}

type processMetrics struct {
	task *task.Task
	once sync.Once
	err  error
}

func newProcessMetrics() *processMetrics {
	return &processMetrics{task: task.RootTask("metrics", false)}
}

func (metrics *processMetrics) Activate() configtypes.ComponentActivation {
	activation := configtypes.ComponentActivation{
		Configured: true,
		Required:   false,
	}
	metrics.once.Do(func() {
		metrics.err = errors.Join(
			systeminfo.Poller.Start(metrics.task),
			uptime.Poller.Start(metrics.task),
		)
	})
	activation.Ready = true
	activation.Err = gperr.Wrap(metrics.err)
	return activation
}

// RuntimeManager owns configuration transitions and process-lifetime runtime
// services. A reload is serialized from preparation through its final report.
type RuntimeManager struct {
	configPath string
	newState   runtimeFactory
	history    *events.History

	reloadMu sync.Mutex
	active   synk.Value[configtypes.State]

	metrics metricsService
}

func NewRuntimeManager(configPath string) *RuntimeManager {
	manager := &RuntimeManager{
		configPath: configPath,
		newState: func() managedState {
			return NewState()
		},
		history: events.NewHistory(),
	}
	metrics := newProcessMetrics()
	configtypes.SetRuntimeStateSource(metrics.task, manager)
	manager.metrics = metrics
	return manager
}

var defaultRuntimeManager = NewRuntimeManager(common.ConfigPath)

func Load(initializeRuntime func(context.Context) error) configtypes.ReloadResult {
	return defaultRuntimeManager.Load(initializeRuntime)
}

func Reload() configtypes.ReloadResult {
	return defaultRuntimeManager.Reload()
}

// Load activates the initial candidate's proxy routes before initializeRuntime
// runs. This lets process initialization reach dependencies routed through
// GoDoxy itself while keeping API listeners unavailable until their
// authentication and handler dependencies are ready.
func (manager *RuntimeManager) Load(initializeRuntime func(context.Context) error) configtypes.ReloadResult {
	return manager.transition(true, initializeRuntime)
}

// Reload accepts a configuration once preparation completes without a
// rejecting issue. After acceptance, the new configuration remains
// authoritative even if providers, routes, or other runtime components fail
// to activate.
//
// Activation failures are returned in ReloadResult and determine runtime
// health; they never restore the previous configuration.
func (manager *RuntimeManager) Reload() configtypes.ReloadResult {
	return manager.transition(false, nil)
}

func (manager *RuntimeManager) RuntimeState() configtypes.State {
	return manager.active.Load()
}

func ShutdownTimeout() int {
	state := defaultRuntimeManager.RuntimeState()
	if state == nil {
		panic("runtime is not loaded")
	}
	return state.Value().TimeoutShutdown
}

// commitState publishes a configuration selected at the acceptance boundary.
//
// Once preparation completes without a rejecting issue, this configuration is
// authoritative. Activation failures after this point degrade the new runtime;
// they do not restore the previous configuration. All independent subsystems
// must still be activated so the final report describes the complete runtime.
func (manager *RuntimeManager) commitState(state configtypes.State) {
	manager.active.Store(state)
}

// BeginRuntimeMutation reserves the same gate used by reload transitions and
// verifies that expected is still authoritative. It deliberately fails rather
// than waiting behind a transition: an old API request must not hold shutdown
// open while waiting to discover that its runtime was replaced.
func (manager *RuntimeManager) BeginRuntimeMutation(expected configtypes.State) (func(), error) {
	if !manager.reloadMu.TryLock() {
		return nil, configtypes.ErrRuntimeTransitioning
	}
	release := manager.reloadMu.Unlock
	active := manager.RuntimeState()
	if expected == nil || active == nil || expected.Task() != active.Task() {
		release()
		return nil, fmt.Errorf("%w: request runtime is no longer authoritative", configtypes.ErrConfigChanged)
	}
	if cause := context.Cause(expected.Context()); cause != nil {
		release()
		return nil, fmt.Errorf("%w: %w", configtypes.ErrConfigChanged, cause)
	}
	return release, nil
}

func (manager *RuntimeManager) transition(initial bool, initializeRuntime func(context.Context) error) configtypes.ReloadResult {
	manager.reloadMu.Lock()
	defer manager.reloadMu.Unlock()

	if initial && manager.RuntimeState() != nil {
		active := manager.RuntimeState()
		return manager.report(active.Context(), configtypes.ReloadResult{
			Committed: false,
			Health:    configtypes.ActivationFailed,
			Issues: []configtypes.ActivationIssue{{
				Component: "runtime",
				Severity:  configtypes.IssueRejecting,
				Err:       gperr.New("configuration already loaded"),
			}},
		})
	}

	candidate := manager.newState()
	events.SetCtx(candidate.Task(), manager.history)
	configtypes.SetRuntimeMutationCoordinator(candidate.Task(), manager)

	initErr := candidate.InitFromFile(manager.configPath)
	issues := candidate.PreparationIssues()
	_, hasRejectingError := errors.AsType[RejectingError](initErr)
	preparationRejected := false
	for _, issue := range issues {
		if issue.Severity.IsFailure() {
			preparationRejected = true
			break
		}
	}
	runtimeCause := context.Cause(candidate.Context())
	if runtimeCause != nil {
		preparationRejected = true
	}

	if preparationRejected || hasRejectingError {
		stopReason := initErr
		if runtimeCause != nil {
			issues = append(issues, configtypes.ActivationIssue{
				Component: "runtime",
				Severity:  configtypes.IssueRejecting,
				Err:       gperr.Wrap(runtimeCause),
			})
			stopReason = runtimeCause
		}
		candidate.discardTmpLog()
		reportCtx := candidate.Context()
		if active := manager.RuntimeState(); active != nil {
			reportCtx = active.Context()
		}
		result := manager.report(reportCtx, configtypes.ReloadResult{
			Committed: false,
			Health:    configtypes.ActivationFailed,
			Issues:    issues,
		})
		candidate.Stop(stopReason)
		return result
	}

	if previous := manager.RuntimeState(); previous != nil {
		previous.Stop(configtypes.ErrConfigChanged)
	}
	candidate.setStatus(configtypes.RuntimeActivating)
	manager.commitState(candidate)

	activationTask := candidate.Task().Subtask("activation", false)
	var report configtypes.ActivationReport
	report.Providers = candidate.ActivateProviders(activationTask)
	if initializeRuntime == nil {
		report.API = candidate.ActivateAPIServers(activationTask)
	} else if err := initializeRuntime(activationTask.Context()); err != nil {
		report.API = blockedAPIActivation(err)
	} else {
		report.API = candidate.ActivateAPIServers(activationTask)
	}
	report.Metrics = manager.metrics.Activate()

	runtimeCause = context.Cause(candidate.Context())
	if runtimeCause != nil {
		report = canceledActivationReport(report, runtimeCause)
		issues = append(issues, configtypes.ActivationIssue{
			Component: "runtime",
			Severity:  configtypes.IssueFailed,
			Err:       gperr.Wrap(runtimeCause),
		})
	}
	issues = append(issues, issuesFromActivation(report)...)

	health := activationHealth(issues, report)
	candidate.setActivation(report, health)
	if err := candidate.FlushTmpLog(); err != nil {
		issues = append(issues, configtypes.ActivationIssue{
			Component: "diagnostics",
			Severity:  configtypes.IssueDegraded,
			Err:       gperr.Wrap(err),
		})
		health = activationHealth(issues, report)
		candidate.setActivation(report, health)
	}

	return manager.report(candidate.Context(), configtypes.ReloadResult{
		Committed:        true,
		Health:           health,
		Issues:           issues,
		ActivationReport: report,
	})
}

func (manager *RuntimeManager) report(ctx context.Context, result configtypes.ReloadResult) configtypes.ReloadResult {
	level := events.LevelInfo
	logEvent := log.Info()
	notifLevel := zerolog.InfoLevel

	switch result.Health {
	case configtypes.ActivationDegraded:
		level = events.LevelWarn
		logEvent = log.Warn()
		notifLevel = zerolog.WarnLevel
	case configtypes.ActivationFailed:
		level = events.LevelError
		logEvent = log.Error()
		notifLevel = zerolog.ErrorLevel
	}

	combinedErr := result.IssueError()
	commitment := "rejected"
	if result.Committed {
		commitment = "committed"
	}
	message := fmt.Sprintf(
		"config %s: %s\nproviders: %d ready, %d degraded, %d failed\nroutes: %d active, %d failed\napi: main %s, local %s\nmetrics: %s",
		commitment,
		result.Health,
		result.Providers.ReadyProviders,
		result.Providers.DegradedProviders,
		result.Providers.FailedProviders,
		result.Providers.ActiveRoutes,
		result.Providers.FailedRoutes,
		componentStatus(result.API.Main),
		componentStatus(result.API.Local),
		componentStatus(result.Metrics),
	)
	if combinedErr != nil {
		// render in nested list
		diagnostics := gperr.NewBuilder("errors:")
		for _, issue := range result.Issues {
			diagnostics.Add(issue.Err)
		}
		logEvent.Msg(message + "\n" + diagnostics.Error().Error())
	} else {
		logEvent.Msg(message)
	}

	manager.history.Add(events.NewEvent(level, "config", "lifecycle", result))

	if combinedErr != nil && ctx != nil {
		notif.FromCtx(ctx).Notify(&notif.LogMessage{
			Level: notifLevel,
			Title: fmt.Sprintf("Configuration lifecycle %s", result.Health),
			Body:  notif.ErrorBody(combinedErr),
		})
	}
	return result
}

func blockedAPIActivation(err error) configtypes.APIActivationReport {
	err = fmt.Errorf("initialize API dependencies: %w", err)
	report := configtypes.APIActivationReport{
		Main: configtypes.ComponentActivation{
			Configured: true,
			Required:   true,
			Err:        gperr.Wrap(err),
		},
		Local: configtypes.ComponentActivation{
			Configured: common.LocalAPIHTTPAddr != "",
		},
	}
	if report.Local.Configured {
		report.Local.Err = gperr.Wrap(err)
	}
	return report
}

func canceledActivationReport(report configtypes.ActivationReport, cause error) configtypes.ActivationReport {
	var providers routing.ProviderActivationReport
	for _, activation := range report.Providers.Providers {
		wasActive := activation.ActiveRoutes > 0 || activation.EventLoopReady
		activation.ActiveRoutes = 0
		activation.EventLoopReady = false
		if wasActive {
			activation.InfrastructureError = gperr.Join(activation.InfrastructureError, cause)
		}
		providers.Add(activation)
	}
	report.Providers = providers

	if report.API.Main.Ready {
		report.API.Main.Ready = false
		report.API.Main.Err = gperr.Join(report.API.Main.Err, cause)
	}
	if report.API.Local.Ready {
		report.API.Local.Ready = false
		report.API.Local.Err = gperr.Join(report.API.Local.Err, cause)
	}
	return report
}

func componentStatus(component configtypes.ComponentActivation) string {
	if !component.Configured {
		return "not configured"
	}
	if component.Ready && component.Err == nil {
		return "ready"
	}
	if component.Ready {
		return "ready with warnings"
	}
	return "failed"
}

func issuesFromActivation(report configtypes.ActivationReport) []configtypes.ActivationIssue {
	issues := make([]configtypes.ActivationIssue, 0, report.Providers.FailedRoutes+report.Providers.FailedProviders+3)
	for _, provider := range report.Providers.Providers {
		for _, route := range provider.FailedRoutes {
			issues = append(issues, configtypes.ActivationIssue{
				Component: "route",
				Subject:   route.Route,
				Severity:  configtypes.IssueDegraded,
				Err:       route.Err,
			})
		}
		if provider.InfrastructureError != nil {
			issues = append(issues, configtypes.ActivationIssue{
				Component: "provider",
				Subject:   provider.Provider,
				Severity:  configtypes.IssueDegraded,
				Err:       provider.InfrastructureError,
			})
		}
	}
	components := [...]struct {
		name       string
		activation configtypes.ComponentActivation
	}{
		{name: "api", activation: report.API.Main},
		{name: "local_api", activation: report.API.Local},
		{name: "metrics", activation: report.Metrics},
	}
	for _, component := range components {
		if component.activation.Err == nil {
			continue
		}
		severity := configtypes.IssueDegraded
		if component.activation.Required {
			severity = configtypes.IssueFailed
		}
		issues = append(issues, configtypes.ActivationIssue{
			Component: component.name,
			Severity:  severity,
			Err:       component.activation.Err,
		})
	}
	return issues
}

func activationHealth(issues []configtypes.ActivationIssue, report configtypes.ActivationReport) configtypes.ActivationHealth {
	failedIssue := false
	for _, issue := range issues {
		if issue.Severity.IsFailure() {
			failedIssue = true
			break
		}
	}
	if report.Providers.AllFailed() || failedIssue {
		return configtypes.ActivationFailed
	}
	if report.Providers.DegradedProviders > 0 || report.Providers.FailedProviders > 0 || len(issues) > 0 {
		return configtypes.ActivationDegraded
	}
	return configtypes.ActivationHealthy
}

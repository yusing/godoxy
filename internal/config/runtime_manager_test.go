package config

import (
	"context"
	"errors"
	"iter"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/routing"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/events"
	"github.com/yusing/goutils/server"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
)

type lifecycleTestMetrics struct {
	activation configtypes.ComponentActivation
	calls      atomic.Int32
}

type lifecycleTestNotifier struct {
	mu       sync.Mutex
	messages []*notif.LogMessage
}

func (notifier *lifecycleTestNotifier) Notify(message *notif.LogMessage) {
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	notifier.messages = append(notifier.messages, message)
}

func (notifier *lifecycleTestNotifier) Len() int {
	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	return len(notifier.messages)
}

func (metrics *lifecycleTestMetrics) Activate() configtypes.ComponentActivation {
	metrics.calls.Add(1)
	return metrics.activation
}

type lifecycleTestState struct {
	cfg  configtypes.Config
	task *task.Task

	initErr error
	issues  []configtypes.ActivationIssue
	initFn  func(*lifecycleTestState) error

	providers           routing.ProviderActivationReport
	api                 configtypes.APIActivationReport
	activateProvidersFn func(task.Parent, *lifecycleTestState) routing.ProviderActivationReport
	activateAPIFn       func(task.Parent, *lifecycleTestState) configtypes.APIActivationReport

	providerCalls atomic.Int32
	apiCalls      atomic.Int32
	flushCalls    atomic.Int32
	stopCalls     atomic.Int32

	mu         sync.Mutex
	status     configtypes.RuntimeStatus
	activation configtypes.ActivationReport
	stopReason any
}

func newLifecycleTestState(name string) *lifecycleTestState {
	state := &lifecycleTestState{
		cfg:    configtypes.DefaultConfig(),
		task:   task.RootTask(name, false),
		status: configtypes.RuntimePreparing,
		api: configtypes.APIActivationReport{
			Main: configtypes.ComponentActivation{Configured: true, Required: true, Ready: true},
		},
	}
	configtypes.SetCtx(state.task, state)
	return state
}

func (state *lifecycleTestState) InitFromFile(string) error {
	if state.initFn != nil {
		return state.initFn(state)
	}
	return state.initErr
}

func (state *lifecycleTestState) Init([]byte) error { return state.InitFromFile("") }
func (state *lifecycleTestState) Task() *task.Task  { return state.task }
func (state *lifecycleTestState) Context() context.Context {
	return state.task.Context()
}
func (state *lifecycleTestState) Value() *configtypes.Config { return &state.cfg }
func (state *lifecycleTestState) Entrypoint() routing.Entrypoint {
	return nil
}
func (state *lifecycleTestState) ShortLinkMatcher() configtypes.ShortLinkMatcher { return nil }
func (state *lifecycleTestState) AutoCertProvider() server.CertProvider          { return nil }
func (state *lifecycleTestState) LoadOrStoreProvider(string, routing.Provider) (routing.Provider, bool) {
	return nil, false
}
func (state *lifecycleTestState) DeleteProvider(string) {}
func (state *lifecycleTestState) IterProviders() iter.Seq2[string, routing.Provider] {
	return func(func(string, routing.Provider) bool) {}
}
func (state *lifecycleTestState) NumProviders() int { return len(state.providers.Providers) }
func (state *lifecycleTestState) ActivateProviders(parent task.Parent) routing.ProviderActivationReport {
	state.providerCalls.Add(1)
	if state.activateProvidersFn != nil {
		return state.activateProvidersFn(parent, state)
	}
	return state.providers
}

func (state *lifecycleTestState) FlushTmpLog() error {
	state.flushCalls.Add(1)
	return nil
}

func (state *lifecycleTestState) ActivateAPIServers(parent task.Parent) configtypes.APIActivationReport {
	state.apiCalls.Add(1)
	if state.activateAPIFn != nil {
		return state.activateAPIFn(parent, state)
	}
	return state.api
}

func (state *lifecycleTestState) RuntimeSnapshot() configtypes.RuntimeSnapshot {
	state.mu.Lock()
	defer state.mu.Unlock()
	return configtypes.RuntimeSnapshot{Status: state.status, Activation: state.activation}
}

func (state *lifecycleTestState) Stop(reason any) {
	state.stopCalls.Add(1)
	state.mu.Lock()
	state.status = configtypes.RuntimeStopping
	state.stopReason = reason
	state.mu.Unlock()
	state.task.FinishAndWait(reason)
}

func (state *lifecycleTestState) PreparationIssues() []configtypes.ActivationIssue {
	return append([]configtypes.ActivationIssue(nil), state.issues...)
}
func (state *lifecycleTestState) discardTmpLog() {}
func (state *lifecycleTestState) setStatus(status configtypes.RuntimeStatus) {
	state.mu.Lock()
	state.status = status
	state.mu.Unlock()
}

func (state *lifecycleTestState) setActivation(report configtypes.ActivationReport, health configtypes.ActivationHealth) {
	state.mu.Lock()
	state.activation = report
	switch health {
	case configtypes.ActivationHealthy:
		state.status = configtypes.RuntimeHealthy
	case configtypes.ActivationDegraded:
		state.status = configtypes.RuntimeDegraded
	case configtypes.ActivationFailed:
		state.status = configtypes.RuntimeFailed
	}
	state.mu.Unlock()
}

func setCommittedState(t *testing.T, manager *RuntimeManager, state configtypes.State) {
	t.Helper()
	previous := manager.active.Load()
	manager.active = synk.Value[configtypes.State]{}
	if state != nil {
		manager.active.Store(state)
	}
	t.Cleanup(func() {
		current := manager.active.Load()
		if current != nil && current != previous {
			current.Stop(nil)
		}
		manager.active = synk.Value[configtypes.State]{}
		if previous != nil {
			manager.active.Store(previous)
		}
	})
}

func lifecycleTestManager(candidate managedState, metrics *lifecycleTestMetrics) *RuntimeManager {
	return &RuntimeManager{
		configPath: "test.yml",
		newState:   func() managedState { return candidate },
		history:    events.NewHistory(),
		metrics:    metrics,
	}
}

func TestRuntimeManagerProvidesCurrentRuntimeToProcessMetrics(t *testing.T) {
	manager := NewRuntimeManager("unused.yml")
	setCommittedState(t, manager, nil)
	metrics, ok := manager.metrics.(*processMetrics)
	require.True(t, ok)

	first := newLifecycleTestState("metrics-runtime-first")
	t.Cleanup(func() { first.Stop(nil) })
	manager.commitState(first)
	require.Same(t, first, configtypes.RuntimeStateFromCtx(metrics.task.Context()))

	second := newLifecycleTestState("metrics-runtime-second")
	manager.commitState(second)
	require.Same(t, second, configtypes.RuntimeStateFromCtx(metrics.task.Context()))
}

func TestRuntimeManagerCommitsHealthyInitialLoad(t *testing.T) {
	candidate := newLifecycleTestState("candidate-initial")
	metrics := &lifecycleTestMetrics{activation: configtypes.ComponentActivation{Configured: true, Ready: true}}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, nil)
	var initializerCalls atomic.Int32

	result := manager.Load(func(context.Context) error {
		initializerCalls.Add(1)
		require.EqualValues(t, 1, candidate.providerCalls.Load(), "routes must be active before runtime initialization")
		require.Zero(t, candidate.apiCalls.Load(), "API listeners must wait for runtime initialization")
		require.Zero(t, metrics.calls.Load())
		return nil
	})

	require.True(t, result.Committed)
	require.Equal(t, configtypes.ActivationHealthy, result.Health)
	require.Same(t, candidate, manager.RuntimeState())
	require.EqualValues(t, 1, initializerCalls.Load())
	require.EqualValues(t, 1, candidate.providerCalls.Load())
	require.EqualValues(t, 1, candidate.apiCalls.Load())
	require.EqualValues(t, 1, metrics.calls.Load())
	require.Len(t, manager.history.Get(), 1)
}

func TestRuntimeManagerDoesNotStartAPIsWhenInitialRuntimeInitializationFails(t *testing.T) {
	candidate := newLifecycleTestState("candidate-initialization-failure")
	candidate.providers.Add(routing.ProviderActivation{
		Provider:        "self-hosted-identity",
		ActiveRoutes:    1,
		DesiredRoutes:   1,
		AttemptedRoutes: 1,
		EventLoopReady:  true,
	})
	metrics := &lifecycleTestMetrics{activation: configtypes.ComponentActivation{Configured: true, Ready: true}}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, nil)
	identityErr := errors.New("OIDC discovery unavailable")

	result := manager.Load(func(context.Context) error {
		require.EqualValues(t, 1, candidate.providerCalls.Load())
		require.Zero(t, candidate.apiCalls.Load())
		return identityErr
	})

	require.True(t, result.Committed)
	require.Equal(t, configtypes.ActivationFailed, result.Health)
	require.Same(t, candidate, manager.RuntimeState())
	require.Equal(t, 1, result.Providers.ActiveRoutes)
	require.Zero(t, candidate.apiCalls.Load())
	require.EqualValues(t, 1, metrics.calls.Load())
	require.True(t, result.API.Main.Configured)
	require.True(t, result.API.Main.Required)
	require.False(t, result.API.Main.Ready)
	require.ErrorIs(t, result.API.Main.Err, identityErr)
	require.ErrorIs(t, result.IssueError(), identityErr)

	snapshot := candidate.RuntimeSnapshot()
	require.Equal(t, configtypes.RuntimeFailed, snapshot.Status)
	require.False(t, snapshot.Activation.API.Main.Ready)
	require.ErrorIs(t, snapshot.Activation.API.Main.Err, identityErr)
}

func TestRuntimeManagerCancelsInitialRuntimeInitializationWithCandidate(t *testing.T) {
	candidate := newLifecycleTestState("candidate-canceled-initialization")
	candidate.providers.Add(routing.ProviderActivation{
		Provider:        "self-hosted-identity",
		ActiveRoutes:    1,
		DesiredRoutes:   1,
		AttemptedRoutes: 1,
		EventLoopReady:  true,
	})
	metrics := &lifecycleTestMetrics{activation: configtypes.ComponentActivation{Configured: true, Ready: true}}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, nil)
	initializationStarted := make(chan struct{})

	resultCh := make(chan configtypes.ReloadResult, 1)
	go func() {
		resultCh <- manager.Load(func(ctx context.Context) error {
			close(initializationStarted)
			<-ctx.Done()
			return context.Cause(ctx)
		})
	}()
	<-initializationStarted
	candidate.task.Finish(context.Canceled)

	var result configtypes.ReloadResult
	select {
	case result = <-resultCh:
	case <-time.After(time.Second):
		t.Fatal("initial runtime initialization did not stop with its candidate task")
	}

	require.True(t, result.Committed)
	require.Equal(t, configtypes.ActivationFailed, result.Health)
	require.Same(t, candidate, manager.RuntimeState())
	require.ErrorIs(t, result.IssueError(), context.Canceled)
	require.Zero(t, candidate.apiCalls.Load())
	require.EqualValues(t, 1, metrics.calls.Load())
	require.Zero(t, result.Providers.ActiveRoutes)
	require.False(t, result.API.Main.Ready)
}

func TestRuntimeManagerCommitsHealthyReload(t *testing.T) {
	oldState := newLifecycleTestState("old-runtime-healthy")
	candidate := newLifecycleTestState("candidate-runtime-healthy")
	metrics := &lifecycleTestMetrics{activation: configtypes.ComponentActivation{Configured: true, Ready: true}}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, oldState)

	result := manager.Reload()

	require.True(t, result.Committed)
	require.Equal(t, configtypes.ActivationHealthy, result.Health)
	require.Same(t, candidate, manager.RuntimeState())
	require.EqualValues(t, 1, oldState.stopCalls.Load())
	require.EqualValues(t, 1, candidate.providerCalls.Load())
	require.EqualValues(t, 1, candidate.apiCalls.Load())
	require.EqualValues(t, 1, metrics.calls.Load())
}

func TestRuntimeManagerCommitsPartialActivationAndAttemptsEverySubsystem(t *testing.T) {
	oldState := newLifecycleTestState("old-runtime")
	candidate := newLifecycleTestState("candidate-runtime")
	routeErr := errors.New("route listener occupied")
	candidate.providers.Add(routing.ProviderActivation{
		Provider:        "docker.local",
		DesiredRoutes:   2,
		AttemptedRoutes: 2,
		ActiveRoutes:    1,
		FailedRoutes: []routing.RouteActivationIssue{{
			Route: "colliding-route",
			Err:   gperr.Wrap(routeErr),
		}},
		EventLoopReady: true,
	})
	metrics := &lifecycleTestMetrics{activation: configtypes.ComponentActivation{Configured: true, Ready: true}}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, oldState)
	oldNotifier := new(lifecycleTestNotifier)
	candidateNotifier := new(lifecycleTestNotifier)
	notif.SetCtx(oldState.task, oldNotifier)
	notif.SetCtx(candidate.task, candidateNotifier)

	result := manager.Reload()

	require.True(t, result.Committed)
	require.Equal(t, configtypes.ActivationDegraded, result.Health)
	require.Same(t, candidate, manager.RuntimeState())
	require.EqualValues(t, 1, oldState.stopCalls.Load())
	require.ErrorIs(t, oldState.stopReason.(error), configtypes.ErrConfigChanged)
	require.EqualValues(t, 1, candidate.providerCalls.Load())
	require.EqualValues(t, 1, candidate.apiCalls.Load())
	require.EqualValues(t, 1, metrics.calls.Load())
	require.Equal(t, 1, result.Providers.ActiveRoutes)
	require.Equal(t, 1, result.Providers.FailedRoutes)
	require.ErrorIs(t, result.Issues[0].Err, routeErr)
	require.Len(t, manager.history.Get(), 1)
	require.Zero(t, oldNotifier.Len())
	require.Equal(t, 1, candidateNotifier.Len())
}

func TestRuntimeManagerKeepsCommittedRuntimeWhenAllRoutesFail(t *testing.T) {
	oldState := newLifecycleTestState("old-runtime-all-fail")
	candidate := newLifecycleTestState("candidate-runtime-all-fail")
	candidate.providers.Add(routing.ProviderActivation{
		Provider:        "file.routes",
		DesiredRoutes:   1,
		AttemptedRoutes: 1,
		FailedRoutes: []routing.RouteActivationIssue{{
			Route: "only-route",
			Err:   gperr.New("address already in use"),
		}},
		EventLoopReady: true,
	})
	metrics := &lifecycleTestMetrics{activation: configtypes.ComponentActivation{Configured: true, Ready: true}}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, oldState)

	result := manager.Reload()

	require.True(t, result.Committed)
	require.Equal(t, configtypes.ActivationFailed, result.Health)
	require.Same(t, candidate, manager.RuntimeState())
	require.True(t, result.API.Main.Ready)
	require.True(t, result.Metrics.Ready)
	require.EqualValues(t, 1, candidate.apiCalls.Load())
	require.EqualValues(t, 1, metrics.calls.Load())
}

func TestRuntimeManagerDegradesWhenOneProviderFailsAndAnotherServes(t *testing.T) {
	oldState := newLifecycleTestState("old-runtime-provider-partial")
	candidate := newLifecycleTestState("candidate-runtime-provider-partial")
	providerErr := errors.New("docker provider unavailable")
	candidate.providers.Add(routing.ProviderActivation{
		Provider:            "docker.failed",
		InfrastructureError: gperr.Wrap(providerErr),
	})
	candidate.providers.Add(routing.ProviderActivation{
		Provider:        "file.ready",
		DesiredRoutes:   1,
		AttemptedRoutes: 1,
		ActiveRoutes:    1,
		EventLoopReady:  true,
	})
	metrics := &lifecycleTestMetrics{activation: configtypes.ComponentActivation{Configured: true, Ready: true}}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, oldState)

	result := manager.Reload()

	require.True(t, result.Committed)
	require.Equal(t, configtypes.ActivationDegraded, result.Health)
	require.Same(t, candidate, manager.RuntimeState())
	require.Equal(t, 1, result.Providers.ReadyProviders)
	require.Equal(t, 1, result.Providers.FailedProviders)
	require.Equal(t, 1, result.Providers.ActiveRoutes)
	require.Equal(t, 0, result.Providers.FailedRoutes)
	require.Len(t, result.Issues, 1)
	require.Equal(t, configtypes.IssueDegraded, result.Issues[0].Severity)
	require.ErrorIs(t, result.Issues[0].Err, providerErr)
	require.EqualValues(t, 1, candidate.apiCalls.Load())
	require.EqualValues(t, 1, metrics.calls.Load())
}

func TestRuntimeManagerRejectsWrappedErrorWithoutFlattening(t *testing.T) {
	oldState := newLifecycleTestState("old-runtime-rejection")
	candidate := newLifecycleTestState("candidate-runtime-rejection")
	sentinel := errors.New("semantic validation failed")
	wrapped := RejectingError{err: errors.Join(errors.New("invalid config"), sentinel)}
	candidate.initErr = wrapped
	candidate.issues = []configtypes.ActivationIssue{{
		Component: "config",
		Severity:  configtypes.IssueRejecting,
		Err:       gperr.Wrap(wrapped),
	}}
	metrics := &lifecycleTestMetrics{}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, oldState)
	oldNotifier := new(lifecycleTestNotifier)
	candidateNotifier := new(lifecycleTestNotifier)
	notif.SetCtx(oldState.task, oldNotifier)
	notif.SetCtx(candidate.task, candidateNotifier)

	result := manager.Reload()

	require.False(t, result.Committed)
	require.Equal(t, configtypes.ActivationFailed, result.Health)
	require.Same(t, oldState, manager.RuntimeState())
	require.ErrorIs(t, result.Issues[0].Err, sentinel)
	require.EqualValues(t, 0, oldState.stopCalls.Load())
	require.EqualValues(t, 1, candidate.stopCalls.Load())
	require.Zero(t, candidate.providerCalls.Load())
	require.Zero(t, candidate.apiCalls.Load())
	require.Zero(t, metrics.calls.Load())
	require.Equal(t, 1, oldNotifier.Len())
	require.Zero(t, candidateNotifier.Len())

	encoded, err := strutils.MarshalJSON(result)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "semantic validation failed")
}

func TestRuntimeManagerDoesNotClassifyErrorsByMessage(t *testing.T) {
	oldState := newLifecycleTestState("old-runtime-collision")
	candidate := newLifecycleTestState("candidate-runtime-collision")
	rejectingSentinel := errors.New("same message")
	unrelated := errors.New("same message")
	candidate.initErr = unrelated
	candidate.issues = []configtypes.ActivationIssue{{
		Component: "provider",
		Severity:  configtypes.IssueDegraded,
		Err:       gperr.Wrap(unrelated),
	}}
	metrics := &lifecycleTestMetrics{activation: configtypes.ComponentActivation{Configured: true, Ready: true}}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, oldState)

	result := manager.Reload()

	require.True(t, result.Committed)
	require.Equal(t, configtypes.ActivationDegraded, result.Health)
	require.ErrorIs(t, result.Issues[0].Err, unrelated)
	require.NotErrorIs(t, result.Issues[0].Err, rejectingSentinel)
}

func TestRuntimeManagerFailsClosedForUnknownPreparationSeverity(t *testing.T) {
	oldState := newLifecycleTestState("old-runtime-unknown")
	candidate := newLifecycleTestState("candidate-runtime-unknown")
	unknownErr := errors.New("future preparation policy")
	candidate.issues = []configtypes.ActivationIssue{{
		Component: "future-component",
		Severity:  configtypes.IssueSeverity("future"),
		Err:       gperr.Wrap(unknownErr),
	}}
	metrics := &lifecycleTestMetrics{}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, oldState)

	result := manager.Reload()

	require.False(t, result.Committed)
	require.Same(t, oldState, manager.RuntimeState())
	require.ErrorIs(t, result.Issues[0].Err, unknownErr)
	require.Zero(t, metrics.calls.Load())
}

func TestRuntimeManagerCancellationBeforeAcceptanceRetainsPreviousRuntime(t *testing.T) {
	oldState := newLifecycleTestState("old-runtime-cancel-before")
	candidate := newLifecycleTestState("candidate-runtime-cancel-before")
	preparationStarted := make(chan struct{})
	candidate.initFn = func(state *lifecycleTestState) error {
		close(preparationStarted)
		<-state.Context().Done()
		return context.Cause(state.Context())
	}
	metrics := &lifecycleTestMetrics{}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, oldState)

	resultCh := make(chan configtypes.ReloadResult, 1)
	go func() {
		resultCh <- manager.Reload()
	}()
	<-preparationStarted
	candidate.task.Finish(context.Canceled)
	result := <-resultCh

	require.False(t, result.Committed)
	require.Same(t, oldState, manager.RuntimeState())
	require.Equal(t, configtypes.ActivationFailed, result.Health)
	require.ErrorIs(t, result.Issues[len(result.Issues)-1].Err, context.Canceled)
	require.Zero(t, oldState.stopCalls.Load())
	require.EqualValues(t, 1, candidate.stopCalls.Load())
}

func TestRuntimeManagerClassifiesMainAndLocalAPIFailuresIndependently(t *testing.T) {
	mainErr := errors.New("primary API address already in use")
	localErr := errors.New("local API address already in use")
	tests := []struct {
		name   string
		api    configtypes.APIActivationReport
		health configtypes.ActivationHealth
		err    error
	}{
		{
			name: "optional local API failure degrades",
			api: configtypes.APIActivationReport{
				Main:  configtypes.ComponentActivation{Configured: true, Required: true, Ready: true},
				Local: configtypes.ComponentActivation{Configured: true, Err: gperr.Wrap(localErr)},
			},
			health: configtypes.ActivationDegraded,
			err:    localErr,
		},
		{
			name: "required main API failure fails",
			api: configtypes.APIActivationReport{
				Main:  configtypes.ComponentActivation{Configured: true, Required: true, Err: gperr.Wrap(mainErr)},
				Local: configtypes.ComponentActivation{Configured: true, Ready: true},
			},
			health: configtypes.ActivationFailed,
			err:    mainErr,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldState := newLifecycleTestState("old-runtime-api")
			candidate := newLifecycleTestState("candidate-runtime-api")
			candidate.api = test.api
			metrics := &lifecycleTestMetrics{activation: configtypes.ComponentActivation{Configured: true, Ready: true}}
			manager := lifecycleTestManager(candidate, metrics)
			setCommittedState(t, manager, oldState)

			result := manager.Reload()

			require.True(t, result.Committed)
			require.Equal(t, test.health, result.Health)
			require.Same(t, candidate, manager.RuntimeState())
			require.ErrorIs(t, result.Issues[0].Err, test.err)
			require.Equal(t, test.api.Main, result.API.Main)
			require.Equal(t, test.api.Local, result.API.Local)
			require.EqualValues(t, 1, candidate.providerCalls.Load())
			require.EqualValues(t, 1, candidate.apiCalls.Load())
			require.EqualValues(t, 1, metrics.calls.Load())
		})
	}
}

func TestRuntimeManagerPublishesStoppedResourcesAsNotReadyAfterCandidateCancellation(t *testing.T) {
	oldState := newLifecycleTestState("old-runtime-cancel-during-activation")
	candidate := newLifecycleTestState("candidate-runtime-cancel-during-activation")
	candidate.providers.Add(routing.ProviderActivation{
		Provider:        "ready-before-cancel",
		DesiredRoutes:   1,
		AttemptedRoutes: 1,
		ActiveRoutes:    1,
		EventLoopReady:  true,
	})
	activationStarted := make(chan struct{})
	candidate.activateAPIFn = func(parent task.Parent, _ *lifecycleTestState) configtypes.APIActivationReport {
		close(activationStarted)
		<-parent.Context().Done()
		// Model a sibling that captured readiness before cancellation and
		// returned it only after the common activation owner was stopped.
		return configtypes.APIActivationReport{
			Main: configtypes.ComponentActivation{
				Configured: true,
				Required:   true,
				Ready:      true,
			},
		}
	}
	metrics := &lifecycleTestMetrics{
		activation: configtypes.ComponentActivation{Configured: true, Ready: true},
	}
	manager := lifecycleTestManager(candidate, metrics)
	setCommittedState(t, manager, oldState)

	resultCh := make(chan configtypes.ReloadResult, 1)
	go func() {
		resultCh <- manager.Reload()
	}()
	<-activationStarted
	candidate.task.Finish(context.Canceled)

	var result configtypes.ReloadResult
	select {
	case result = <-resultCh:
	case <-time.After(time.Second):
		t.Fatal("reload did not finish after candidate cancellation")
	}

	require.True(t, result.Committed)
	require.Equal(t, configtypes.ActivationFailed, result.Health)
	require.Same(t, candidate, manager.RuntimeState())
	require.ErrorIs(t, result.IssueError(), context.Canceled)
	require.Zero(t, result.Providers.ActiveRoutes)
	require.Zero(t, result.Providers.ReadyProviders)
	require.Equal(t, 1, result.Providers.FailedProviders)
	require.False(t, result.Providers.Providers[0].EventLoopReady)
	require.ErrorIs(t, result.Providers.Providers[0].InfrastructureError, context.Canceled)
	require.False(t, result.API.Main.Ready)
	require.ErrorIs(t, result.API.Main.Err, context.Canceled)
	require.True(t, result.Metrics.Ready)

	snapshot := candidate.RuntimeSnapshot()
	require.Equal(t, configtypes.RuntimeFailed, snapshot.Status)
	require.Zero(t, snapshot.Activation.Providers.ActiveRoutes)
	require.Zero(t, snapshot.Activation.Providers.ReadyProviders)
	require.False(t, snapshot.Activation.API.Main.Ready)
	require.True(t, snapshot.Activation.Metrics.Ready)
}

func TestRuntimeManagerMutationLeaseRejectsOverlapAndStaleRuntime(t *testing.T) {
	active := newLifecycleTestState("mutation-active")
	stale := newLifecycleTestState("mutation-stale")
	manager := lifecycleTestManager(active, &lifecycleTestMetrics{})
	setCommittedState(t, manager, active)

	release, err := manager.BeginRuntimeMutation(active)
	require.NoError(t, err)
	require.NotNil(t, release)

	_, err = manager.BeginRuntimeMutation(active)
	require.ErrorIs(t, err, configtypes.ErrRuntimeTransitioning)
	release()

	_, err = manager.BeginRuntimeMutation(stale)
	require.ErrorIs(t, err, configtypes.ErrConfigChanged)

	release, err = manager.BeginRuntimeMutation(active)
	require.NoError(t, err)
	release()
}

func TestRuntimeManagerRejectsMalformedConfigurationAndRetainsRuntime(t *testing.T) {
	oldState := newLifecycleTestState("old-runtime-malformed")

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("entrypoint: ["), 0o600))
	metrics := &lifecycleTestMetrics{}
	manager := NewRuntimeManager(path)
	manager.metrics = metrics
	setCommittedState(t, manager, oldState)

	result := manager.Reload()

	require.False(t, result.Committed)
	require.Equal(t, configtypes.ActivationFailed, result.Health)
	require.Same(t, oldState, manager.RuntimeState())
	require.NotEmpty(t, result.Issues)
	require.Zero(t, metrics.calls.Load())
}

func TestStateReportsProviderConstructionFailureInActivationCounts(t *testing.T) {
	state := NewState()
	t.Cleanup(func() { state.Stop(nil) })
	state.Config = configtypes.DefaultConfig()
	state.WebUI.Aliases = []string{""}
	state.Providers.Files = []string{"missing-runtime-provider.yml"}

	err := state.loadRouteProviders()
	require.Error(t, err)

	report := state.ActivateProviders(state.Task())
	require.Len(t, report.Providers, 1)
	require.Equal(t, 1, report.FailedProviders)
	require.Equal(t, 0, report.ReadyProviders)
	require.Equal(t, "missing-runtime-provider.yml", report.Providers[0].Provider)
	require.Error(t, report.Providers[0].InfrastructureError)
}

func TestActivationHealthFailsForUnknownFutureIssueAndAcceptsEmptyProvider(t *testing.T) {
	empty := routing.ProviderActivationReport{}
	empty.Add(routing.ProviderActivation{Provider: "future-dynamic", EventLoopReady: true})
	require.False(t, empty.AllFailed())
	require.Equal(t, 1, empty.ReadyProviders)
	require.Equal(t, configtypes.ActivationHealthy, activationHealth(nil, configtypes.ActivationReport{Providers: empty}))

	issues := []configtypes.ActivationIssue{{
		Component: "future-component",
		Severity:  configtypes.IssueSeverity("future"),
		Err:       gperr.New("unknown activation policy"),
	}}
	require.Equal(t, configtypes.ActivationFailed, activationHealth(issues, configtypes.ActivationReport{}))
}

var _ managedState = (*lifecycleTestState)(nil)

package config

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/common"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/net/hostaddr"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/route/provider"
)

const addressSetsConfig = `
address_sets:
  family:
    - 10.0.0.0/8
    - host:localhost
acl:
  default: deny
  allow:
    - set:family
entrypoint:
  middlewares:
    - use: cidr_whitelist
      allow:
        - set:family
`

func resetPublishedAddressSets(t *testing.T) {
	t.Helper()
	// host: entries resolve in the background; pin them so assertions
	// right after loading are deterministic.
	hostaddr.UseStaticRecordsForTest(t, map[string][]string{"localhost": {"127.0.0.1"}})
	nettypes.PublishAddressSets(nil)
	t.Cleanup(func() { nettypes.PublishAddressSets(nil) })
}

func TestAddressSetsPublishedOnlyWhenAccepted(t *testing.T) {
	resetPublishedAddressSets(t)

	state := NewState()
	t.Cleanup(func() { state.Stop(nil) })
	require.NoError(t, state.Init([]byte(addressSetsConfig)))
	require.Len(t, state.Value().AddressSets["family"], 2)

	ref, err := nettypes.NewAddressSetRef("family")
	require.NoError(t, err)
	require.False(t, ref.Defined(), "a prepared but not accepted config must not publish its sets")

	state.setStatus(config.RuntimeActivating)
	require.True(t, ref.Defined())
	require.True(t, ref.Contains(net.ParseIP("10.1.2.3")))
	require.True(t, ref.Contains(net.ParseIP("127.0.0.1")), "host: entries resolve inside sets")
	require.False(t, ref.Contains(net.ParseIP("203.0.113.7")))
}

func TestAddressSetsRejectedReloadKeepsActiveSets(t *testing.T) {
	resetPublishedAddressSets(t)

	active := NewState()
	t.Cleanup(func() { active.Stop(nil) })
	require.NoError(t, active.Init([]byte(addressSetsConfig)))
	active.setStatus(config.RuntimeActivating)

	candidate := NewState()
	t.Cleanup(func() { candidate.Stop(nil) })
	err := candidate.Init([]byte("address_sets:\n  family:\n    - set:other\n  other:\n    - 192.168.0.0/16\n"))
	require.ErrorContains(t, err, "address sets cannot reference other sets")

	ref, err := nettypes.NewAddressSetRef("family")
	require.NoError(t, err)
	require.True(t, ref.Contains(net.ParseIP("10.1.2.3")))
	require.False(t, ref.Contains(net.ParseIP("192.168.1.1")))
}

func TestAddressSetsValidation(t *testing.T) {
	resetPublishedAddressSets(t)

	for name, doc := range map[string]string{
		"nested set": "address_sets:\n  a:\n    - set:b\n  b:\n    - 10.0.0.0/8\n",
		"bad name":   "address_sets:\n  bad.name:\n    - 10.0.0.0/8\n",
		"bad entry":  "address_sets:\n  a:\n    - not-an-ip\n",
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, config.Validate([]byte(doc)))
		})
	}
	require.NoError(t, config.Validate([]byte(addressSetsConfig)))
}

func TestAddressSetsRejectZeroEntries(t *testing.T) {
	for _, entry := range []string{
		"null", `""`, "{}",
		"{ip: [203, 0, 113, 7], mask: [255]}",
		"{ip: [203], mask: [255, 255, 255, 0]}",
		"{ip: [203, 0, 113, 7], mask: [255, 0, 255, 0]}",
	} {
		t.Run(entry, func(t *testing.T) {
			doc := []byte("address_sets:\n  family:\n    - 10.0.0.0/8\n    - " + entry + "\n")
			require.ErrorContains(t, config.Validate(doc), "address_sets.family[1]")
			candidate := NewState()
			t.Cleanup(func() { candidate.Stop(nil) })
			require.ErrorContains(t, candidate.Init(doc), "address_sets.family[1]")
		})
	}
	for _, entry := range []string{
		"203.0.113.7", "203.0.113.0/24", "2001:db8::7", "2001:db8::/32",
		"::ffff:203.0.113.7/128", "{ip: [203, 0, 113, 7], mask: [255, 255, 255, 0]}",
	} {
		require.NoError(t, config.Validate([]byte("address_sets:\n  family:\n    - "+entry+"\n")), entry)
	}
}

func TestAddressSetReferencesUseCandidateDefinitions(t *testing.T) {
	resetPublishedAddressSets(t)
	var source nettypes.CIDR
	require.NoError(t, source.Parse("10.0.0.0/8"))
	nettypes.PublishAddressSets(map[string][]*nettypes.CIDR{"old": {&source}})

	consumers := map[string]string{
		"acl":               "acl:\n  default: deny\n  allow: [set:family]\n",
		"entrypoint allow":  "entrypoint:\n  middlewares:\n    - use: cidr_whitelist\n      allow: [set:family]\n",
		"entrypoint from":   "entrypoint:\n  middlewares:\n    - use: real_ip\n      header: X-Real-IP\n      from: [set:family]\n",
		"entrypoint bypass": "entrypoint:\n  middlewares:\n    - use: redirect_http\n      bypass: ['!remote set:family | remote 192.168.0.0/16']\n",
		"entrypoint rules":  "entrypoint:\n  rules:\n    not_found: |\n      remote set:family {\n        error 403 Forbidden\n      }\n",
		"access log":        "entrypoint:\n  access_log:\n    stdout: true\n    format: common\n    filters:\n      cidr: {values: ['set:family']}\n",
		"webui":             "webui:\n  middlewares:\n    cidr_whitelist:\n      allow: [set:family]\n",
		"action middleware": "webui:\n  rules: |\n    default {\n      middleware cidr_whitelist {\n        allow: [set:family]\n      }\n    }\n",
	}
	for name, consumer := range consumers {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, config.Validate([]byte(consumer)), nettypes.ErrUndefinedAddressSet)
			candidateDoc := "address_sets:\n  family: [203.0.113.0/24]\n" + consumer
			require.NoError(t, config.Validate([]byte(candidateDoc)), "candidate-local names must work before publication")
			require.ErrorIs(t, config.Validate([]byte("address_sets:\n  old: []\n"+consumer)), nettypes.ErrUndefinedAddressSet)
		})
	}
	// A name present in the previous runtime cannot satisfy a candidate that removes it.
	require.ErrorIs(t, config.Validate([]byte("acl:\n  allow: [set:old]\n")), nettypes.ErrUndefinedAddressSet)
	ref, err := nettypes.NewAddressSetRef("old")
	require.NoError(t, err)
	require.True(t, ref.Contains(net.ParseIP("10.1.2.3")), "validation must leave the active definitions unchanged")
	require.NoError(t, config.Validate([]byte("webui:\n  rules: |\n    default {\n      set header X-Example set:literal\n    }\n")), "literal strings are not references")
}

func TestAddressSetReferencesInIncludedRoutes(t *testing.T) {
	resetPublishedAddressSets(t)
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(common.ConfigBasePath, 0700))
	require.NoError(t, os.WriteFile("rules.txt", []byte("remote set:family { error 403 Forbidden }"), 0600))

	for name, consumer := range map[string]string{
		"middleware":        "  middlewares:\n    cidr_whitelist:\n      allow: [set:family]\n",
		"unavailable agent": "  agent: missing\n  middlewares:\n    cidr_whitelist:\n      allow: [set:family]\n",
		"bypass":            "  middlewares:\n    redirect_http:\n      bypass: ['remote set:family']\n",
		"rules":             "  rules: |\n    remote set:family {\n      error 403 Forbidden\n    }\n",
		"rule file":         "  rule_file: rules.txt\n",
		"nested rules":      "  rules: |\n    default {\n      !remote set:family {\n        error 403 Forbidden\n      }\n    }\n",
		"middleware action": "  rules: |\n    default {\n      middleware cidr_whitelist {\n        allow: [set:family]\n      }\n    }\n",
	} {
		t.Run(name, func(t *testing.T) {
			routeDoc := []byte("test:\n  scheme: http\n  host: 192.0.2.1\n  port: 8080\n" + consumer)
			require.NoError(t, os.WriteFile(filepath.Join(common.ConfigBasePath, "routes.yml"), routeDoc, 0600))
			candidate := NewState()
			t.Cleanup(func() { candidate.Stop(nil) })
			err := candidate.Init([]byte("providers:\n  include: [routes.yml]\n"))
			require.ErrorIs(t, err, nettypes.ErrUndefinedAddressSet)
			require.ErrorAs(t, err, new(RejectingError))
			require.ErrorIs(t, provider.Validate(candidate.Context(), routeDoc), nettypes.ErrUndefinedAddressSet)

			accepted := NewState()
			t.Cleanup(func() { accepted.Stop(nil) })
			err = accepted.Init([]byte("address_sets:\n  family: [203.0.113.0/24]\nproviders:\n  include: [routes.yml]\n"))
			require.NotErrorIs(t, err, nettypes.ErrUndefinedAddressSet)
			require.NoError(t, provider.Validate(accepted.Context(), routeDoc))
		})
	}
}

func TestAddressSetRejectedReloadPreservesRuntime(t *testing.T) {
	resetPublishedAddressSets(t)
	var source nettypes.CIDR
	require.NoError(t, source.Parse("10.0.0.0/8"))
	nettypes.PublishAddressSets(map[string][]*nettypes.CIDR{"active": {&source}})
	oldState := newLifecycleTestState("active-address-sets")
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("address_sets:\n  abandoned: [203.0.113.0/24]\nacl:\n  allow: [set:active]\n"), 0600))
	manager := NewRuntimeManager(path)
	manager.metrics = &lifecycleTestMetrics{}
	setCommittedState(t, manager, oldState)

	result := manager.Reload()
	require.False(t, result.Committed)
	require.Same(t, oldState, manager.RuntimeState())
	require.Zero(t, oldState.stopCalls.Load())
	ref, err := nettypes.NewAddressSetRef("active")
	require.NoError(t, err)
	require.True(t, ref.Contains(net.ParseIP("10.1.2.3")))
	require.ErrorIs(t, nettypes.PublishedAddressSetValidator("abandoned"), nettypes.ErrUndefinedAddressSet)
	require.ErrorIs(t, provider.Validate(t.Context(), []byte("app:\n  middlewares:\n    cidr_whitelist:\n      allow: [set:abandoned]\n")), nettypes.ErrUndefinedAddressSet)
}

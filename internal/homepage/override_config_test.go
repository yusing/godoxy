package homepage_test

import (
	encjson "encoding/json"
	"sync"
	"testing"

	. "github.com/yusing/godoxy/internal/homepage"
	"github.com/yusing/godoxy/internal/homepage/icons"
	strutils "github.com/yusing/goutils/strings"
	expect "github.com/yusing/goutils/testing"
)

// The periodic jsonstore flush marshals the config while the API keeps writing
// to it, so marshaling must hold the lock.
//
// encoding/json is used instead of strutils.MarshalJSON so the test stays on
// the standard library encoder under the race detector.
func TestOverrideConfigMarshalIsConcurrentSafe(t *testing.T) {
	aliases := []string{"a", "b", "c"}

	cfg := &OverrideConfig{}
	cfg.Initialize()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 500 {
			cfg.OverrideItem(aliases[i%len(aliases)], ItemConfig{
				Show: true,
				Icon: icons.NewURL(icons.SourceSelfhSt, "immich", "svg"),
			})
			cfg.IncrementItemClicks("a")
		}
	}()
	go func() {
		defer wg.Done()
		for range 500 {
			_, err := encjson.Marshal(cfg)
			expect.NoError(t, err)
		}
	}()
	wg.Wait()

	data, err := encjson.Marshal(cfg)
	expect.NoError(t, err)

	loaded := &OverrideConfig{}
	loaded.Initialize()
	expect.NoError(t, strutils.UnmarshalJSON(data, loaded))
	expect.Equal(t, loaded.ItemOverrides["a"].Icon.String(), "@selfhst/immich.svg")
}

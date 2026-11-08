package gpu

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeCounter struct {
	value  *uint64
	closed bool
}

func (f *fakeCounter) Read() (uint64, error) { return *f.value, nil }
func (f *fakeCounter) Close() error          { f.closed = true; return nil }

func writePMU(t *testing.T, root, name string, events map[string]string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "events"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "type"), []byte("17\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cpumask"), []byte("0\n"), 0o644))
	for ev, content := range events {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "events", ev), []byte(content+"\n"), 0o644))
	}
}

func TestIntelCollectorBusyPercent(t *testing.T) {
	root := t.TempDir()
	writePMU(t, root, "i915", map[string]string{
		"rcs0-busy":      "config=0x0",
		"rcs0-busy.unit": "ns",
		"vcs0-busy":      "config=0x200000",
		"vcs1-busy":      "config=0x200100",
		"rcs0-wait":      "config=0x1",
	})

	values := map[uint64]*uint64{}
	var opened []*fakeCounter
	c, err := newIntelCollector(root, func(pmuType uint32, config uint64, cpu int) (counter, error) {
		require.Equal(t, uint32(17), pmuType)
		require.Equal(t, 0, cpu)
		v := new(uint64)
		values[config] = v
		fc := &fakeCounter{value: v}
		opened = append(opened, fc)
		return fc, nil
	})
	require.NoError(t, err)
	require.Len(t, opened, 3, "only *-busy events are opened")

	now := time.Unix(1000, 0)
	c.(*intelCollector).now = func() time.Time { return now }

	stats, err := c.Collect()
	require.NoError(t, err)
	require.Empty(t, stats, "first sample has no delta")

	now = now.Add(time.Second)
	*values[0x0] += uint64(250 * time.Millisecond)
	*values[0x200000] += uint64(2 * time.Second) // clamped
	stats, err = c.Collect()
	require.NoError(t, err)
	require.Equal(t, []Stat{{
		Name:    "i915",
		Engines: map[string]float64{"Render/3D": 25, "Video": 100, "Video/1": 0},
	}}, stats)
}

func TestIntelCollectorNotFound(t *testing.T) {
	_, err := newIntelCollector(t.TempDir(), nil)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestIntelCollectorPermissionDeniedClosesCounters(t *testing.T) {
	root := t.TempDir()
	writePMU(t, root, "i915", map[string]string{
		"bcs0-busy": "config=0x100000",
		"rcs0-busy": "config=0x0",
	})

	var opened []*fakeCounter
	_, err := newIntelCollector(root, func(_ uint32, config uint64, _ int) (counter, error) {
		if len(opened) == 1 {
			return nil, syscall.EACCES
		}
		fc := &fakeCounter{value: new(uint64)}
		opened = append(opened, fc)
		return fc, nil
	})
	require.ErrorIs(t, err, syscall.EACCES)
	require.True(t, opened[0].closed)
}

func TestIntelEngineName(t *testing.T) {
	require.Equal(t, "Render/3D", intelEngineName("rcs0"))
	require.Equal(t, "VideoEnhance", intelEngineName("vecs0"))
	require.Equal(t, "Video/1", intelEngineName("vcs1"))
	require.Equal(t, "xyz0", intelEngineName("xyz0"))
}

type failingCounter struct{ fakeCounter }

func (f *failingCounter) Read() (uint64, error) { return 0, syscall.EIO }

func TestIntelCollectorSkipsUnreadableGPU(t *testing.T) {
	root := t.TempDir()
	writePMU(t, root, "i915", map[string]string{"rcs0-busy": "config=0x0"})
	writePMU(t, root, "i915_0000_03_00.0", map[string]string{"rcs0-busy": "config=0x1"})

	good := &fakeCounter{value: new(uint64)}
	c, err := newIntelCollector(root, func(_ uint32, config uint64, _ int) (counter, error) {
		if config == 0x1 {
			return nil, syscall.EACCES
		}
		return good, nil
	})
	require.ErrorIs(t, err, syscall.EACCES)
	require.NotNil(t, c)
	require.Len(t, c.(*intelCollector).gpus, 1)
}

func TestIntelCollectorReadErrorKeepsOtherGPUs(t *testing.T) {
	root := t.TempDir()
	writePMU(t, root, "i915", map[string]string{"rcs0-busy": "config=0x0"})
	writePMU(t, root, "i915_0000_03_00.0", map[string]string{"rcs0-busy": "config=0x1"})

	good := &fakeCounter{value: new(uint64)}
	broken := &failingCounter{}
	c, err := newIntelCollector(root, func(_ uint32, config uint64, _ int) (counter, error) {
		if config == 0x1 {
			return broken, nil
		}
		return good, nil
	})
	require.NoError(t, err)

	now := time.Unix(1000, 0)
	c.(*intelCollector).now = func() time.Time { return now }
	_, err = c.Collect()
	require.ErrorIs(t, err, syscall.EIO)

	now = now.Add(time.Second)
	*good.value += uint64(500 * time.Millisecond)
	stats, err := c.Collect()
	require.ErrorIs(t, err, syscall.EIO)
	require.Equal(t, []Stat{{Name: "i915", Engines: map[string]float64{"Render/3D": 50}}}, stats)
}

func TestIntelCollectorCounterResetReportsZero(t *testing.T) {
	root := t.TempDir()
	writePMU(t, root, "i915", map[string]string{"rcs0-busy": "config=0x0"})
	v := uint64(time.Second)
	c, err := newIntelCollector(root, func(uint32, uint64, int) (counter, error) {
		return &fakeCounter{value: &v}, nil
	})
	require.NoError(t, err)

	now := time.Unix(1000, 0)
	c.(*intelCollector).now = func() time.Time { return now }
	_, err = c.Collect()
	require.NoError(t, err)

	now = now.Add(time.Second)
	v = 0
	stats, err := c.Collect()
	require.NoError(t, err)
	require.Equal(t, []Stat{{Name: "i915", Engines: map[string]float64{"Render/3D": 0}}}, stats)
}

package gpu

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// i915 exposes per-engine busy time (ns) through its perf PMU, the same source
// intel_gpu_top uses. Opening it needs CAP_PERFMON or kernel.perf_event_paranoid <= 0.
const sysfsPMURoot = "/sys/bus/event_source/devices"

var intelEngineClasses = map[string]string{
	"rcs":  "Render/3D",
	"bcs":  "Blitter",
	"vcs":  "Video",
	"vecs": "VideoEnhance",
	"ccs":  "Compute",
}

type (
	counter interface {
		Read() (uint64, error)
		Close() error
	}
	openCounterFunc func(pmuType uint32, config uint64, cpu int) (counter, error)

	intelCollector struct {
		gpus []*intelGPU
		now  func() time.Time
	}
	intelGPU struct {
		name     string
		engines  []*intelEngine
		lastTime time.Time
	}
	intelEngine struct {
		name    string
		counter counter
		last    uint64
	}
)

func newIntelCollector(root string, open openCounterFunc) (Collector, error) {
	dirs, _ := filepath.Glob(filepath.Join(root, "i915*"))
	if len(dirs) == 0 {
		return nil, ErrNotFound
	}
	c := &intelCollector{now: time.Now}
	var errs []error
	for _, dir := range dirs {
		g, err := openIntelGPU(dir, open)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				errs = append(errs, err)
			}
			continue
		}
		c.gpus = append(c.gpus, g)
	}
	if len(c.gpus) == 0 {
		if len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
		return nil, ErrNotFound
	}
	return c, errors.Join(errs...)
}

func openIntelGPU(dir string, open openCounterFunc) (*intelGPU, error) {
	pmuType, err := readUint(filepath.Join(dir, "type"))
	if err != nil {
		return nil, err
	}
	cpu, err := readFirstCPU(filepath.Join(dir, "cpumask"))
	if err != nil {
		return nil, err
	}
	events, err := os.ReadDir(filepath.Join(dir, "events"))
	if err != nil {
		return nil, err
	}

	g := &intelGPU{name: filepath.Base(dir)}
	for _, ev := range events {
		engine, ok := strings.CutSuffix(ev.Name(), "-busy")
		if !ok {
			continue
		}
		config, err := readEventConfig(filepath.Join(dir, "events", ev.Name()))
		if err != nil {
			g.close()
			return nil, err
		}
		cnt, err := open(uint32(pmuType), config, cpu) //nolint:gosec
		if err != nil {
			g.close()
			return nil, fmt.Errorf("open %s %s counter: %w", g.name, engine, err)
		}
		g.engines = append(g.engines, &intelEngine{name: intelEngineName(engine), counter: cnt})
	}
	if len(g.engines) == 0 {
		return nil, ErrNotFound
	}
	slices.SortFunc(g.engines, func(a, b *intelEngine) int { return strings.Compare(a.name, b.name) })
	return g, nil
}

// intelEngineName maps "vcs1" to "Video/1" and "rcs0" to "Render/3D".
func intelEngineName(engine string) string {
	class := strings.TrimRight(engine, "0123456789")
	instance := engine[len(class):]
	name, ok := intelEngineClasses[class]
	if !ok {
		return engine
	}
	if instance == "" || instance == "0" {
		return name
	}
	return name + "/" + instance
}

func (c *intelCollector) Collect() ([]Stat, error) {
	now := c.now()
	stats := make([]Stat, 0, len(c.gpus))
	var errs []error
	for _, g := range c.gpus {
		values := make([]uint64, len(g.engines))
		var err error
		for i, e := range g.engines {
			if values[i], err = e.counter.Read(); err != nil {
				err = fmt.Errorf("read %s %s counter: %w", g.name, e.name, err)
				break
			}
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}

		elapsed := now.Sub(g.lastTime)
		first := g.lastTime.IsZero()
		stat := Stat{Name: g.name, Engines: make(map[string]float64, len(g.engines))}
		for i, e := range g.engines {
			busy := 0.0
			if elapsed > 0 && values[i] >= e.last {
				busy = min(100, float64(values[i]-e.last)/float64(elapsed)*100)
			}
			stat.Engines[e.name] = busy
			e.last = values[i]
		}
		g.lastTime = now
		if !first {
			stats = append(stats, stat)
		}
	}
	return stats, errors.Join(errs...)
}

func (g *intelGPU) close() {
	for _, e := range g.engines {
		_ = e.counter.Close()
	}
}

func readUint(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimSpace(string(b)), 10, 32)
}

// readFirstCPU parses a cpumask like "0" or "0-3,8".
func readFirstCPU(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(b)), ",")
	first, _, _ = strings.Cut(first, "-")
	return strconv.Atoi(first)
}

// readEventConfig parses an event file like "config=0x100002".
func readEventConfig(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for field := range strings.SplitSeq(strings.TrimSpace(string(b)), ",") {
		if v, ok := strings.CutPrefix(field, "config="); ok {
			return strconv.ParseUint(v, 0, 64)
		}
	}
	return 0, fmt.Errorf("no config in %s", path)
}

type perfCounter int

func openPerfCounter(pmuType uint32, config uint64, cpu int) (counter, error) {
	attr := unix.PerfEventAttr{Type: pmuType, Config: config}
	attr.Size = uint32(unsafe.Sizeof(attr))
	fd, err := unix.PerfEventOpen(&attr, -1, cpu, -1, unix.PERF_FLAG_FD_CLOEXEC)
	if err != nil {
		return nil, err
	}
	return perfCounter(fd), nil
}

func (fd perfCounter) Read() (uint64, error) {
	var buf [8]byte
	if _, err := unix.Read(int(fd), buf[:]); err != nil {
		return 0, err
	}
	return binary.NativeEndian.Uint64(buf[:]), nil
}

func (fd perfCounter) Close() error {
	return unix.Close(int(fd))
}

# internal/metrics/gpu

Collects per-engine GPU utilization for the system info poller.

## Overview

`gpu.Collect()` returns one `Stat` per detected GPU with the busy percentage
(0-100) of each engine since the previous call.
Each GPU kind is a `Collector` implementation registered in a per-OS factory
map; collectors are probed once, on first use.

Hosts without a supported GPU report no GPUs silently. A GPU that is present
but not readable (e.g. a container without the needed permission) is skipped
with a single warning logged at probe time; other GPUs are still reported.
Set `GODOXY_METRICS_DISABLE_GPU=true` to skip probing entirely.

### Non-goals

- No GPU memory, clock or power metrics.
- No process-level attribution.

## Collectors

| Kind  | OS    | Source                                                    | Requirement                                            |
| ----- | ----- | --------------------------------------------------------- | ------------------------------------------------------ |
| intel | Linux | i915 perf PMU `*-busy` counters (same as `intel_gpu_top`) | `CAP_PERFMON`, or `kernel.perf_event_paranoid <= 0`  |

The Intel collector reads `/sys/bus/event_source/devices/i915*`, opens one
system-wide perf counter per `<engine>-busy` event and reports
`Δbusy_ns / Δwall_ns`. Engines use `intel_gpu_top` names (`Render/3D`,
`Blitter`, `Video`, `VideoEnhance`, `Compute`, with `/N` for instances > 0).
The first sample only primes the counters. The `xe` driver is not supported yet.

In Docker, a container running as root (such as the agent) can be granted the
capability with:

```yaml
cap_add:
  - PERFMON
```

Docker only passes added capabilities to processes running as root, so this
does not work for the main server's default non-root `user:`. There, either
run as root or set `kernel.perf_event_paranoid` to `0` or lower on the host.

## Key Types

```go
type Stat struct {
    Name    string             `json:"name"`
    Engines map[string]float64 `json:"engines"`
}

type Collector interface {
    Collect() ([]Stat, error)
}
```

## Adding a GPU kind

Implement `Collector`, return `ErrNotFound` from its factory when no GPU of
that kind exists, and register the factory in `factories_<os>.go`.

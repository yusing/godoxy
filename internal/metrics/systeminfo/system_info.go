package systeminfo // import github.com/yusing/go-proxy/internal/metrics/systeminfo

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
	"github.com/shirou/gopsutil/v4/warning"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/metrics/period"
)

// json tags are left for tests

type (
	Sensors    []sensors.TemperatureStat // @name Sensors
	Aggregated struct {
		Entries []map[string]any
		Mode    SystemInfoAggregateMode
	}
	AggregatedJSON []map[string]any
)

type SystemInfo struct {
	Timestamp  int64                           `json:"timestamp"`
	CPUAverage *float64                        `json:"cpu_average"`
	Memory     *mem.VirtualMemoryStat          `json:"memory"`
	Disks      map[string]*disk.UsageStat      `json:"disks"`    // disk usage by partition
	DisksIO    map[string]*disk.IOCountersStat `json:"disks_io"` // disk IO by device
	Network    *net.IOCountersStat             `json:"network"`
	Sensors    Sensors                         `json:"sensors"` // sensor temperature by key
} // @name SystemInfo

type SystemInfoAggregateMode string // @name SystemInfoAggregateMode

const (
	SystemInfoAggregateModeCPUAverage         SystemInfoAggregateMode = "cpu_average"          // @name SystemInfoAggregateModeCPUAverage
	SystemInfoAggregateModeMemoryUsage        SystemInfoAggregateMode = "memory_usage"         // @name SystemInfoAggregateModeMemoryUsage
	SystemInfoAggregateModeMemoryUsagePercent SystemInfoAggregateMode = "memory_usage_percent" // @name SystemInfoAggregateModeMemoryUsagePercent
	SystemInfoAggregateModeDisksReadSpeed     SystemInfoAggregateMode = "disks_read_speed"     // @name SystemInfoAggregateModeDisksReadSpeed
	SystemInfoAggregateModeDisksWriteSpeed    SystemInfoAggregateMode = "disks_write_speed"    // @name SystemInfoAggregateModeDisksWriteSpeed
	SystemInfoAggregateModeDisksIOPS          SystemInfoAggregateMode = "disks_iops"           // @name SystemInfoAggregateModeDisksIOPS
	SystemInfoAggregateModeDiskUsage          SystemInfoAggregateMode = "disk_usage"           // @name SystemInfoAggregateModeDiskUsage
	SystemInfoAggregateModeNetworkSpeed       SystemInfoAggregateMode = "network_speed"        // @name SystemInfoAggregateModeNetworkSpeed
	SystemInfoAggregateModeNetworkTransfer    SystemInfoAggregateMode = "network_transfer"     // @name SystemInfoAggregateModeNetworkTransfer
	SystemInfoAggregateModeSensorTemperature  SystemInfoAggregateMode = "sensor_temperature"   // @name SystemInfoAggregateModeSensorTemperature
)

var allQueries = []SystemInfoAggregateMode{
	SystemInfoAggregateModeCPUAverage,
	SystemInfoAggregateModeMemoryUsage,
	SystemInfoAggregateModeMemoryUsagePercent,
	SystemInfoAggregateModeDisksReadSpeed,
	SystemInfoAggregateModeDisksWriteSpeed,
	SystemInfoAggregateModeDisksIOPS,
	SystemInfoAggregateModeDiskUsage,
	SystemInfoAggregateModeNetworkSpeed,
	SystemInfoAggregateModeNetworkTransfer,
	SystemInfoAggregateModeSensorTemperature,
}

var Poller = period.NewPoller("system_info", getSystemInfo, aggregate)

func isNoDataAvailable(err error) bool {
	return errors.Is(err, syscall.ENODATA)
}

func getSystemInfo(ctx context.Context, lastResult *SystemInfo) (*SystemInfo, error) {
	errs := gperr.NewBuilderWithConcurrency("failed to get system info")
	var s SystemInfo
	s.Timestamp = time.Now().Unix()

	var wg sync.WaitGroup

	if !common.MetricsDisableCPU {
		wg.Go(func() {
			errs.Add(s.collectCPUInfo(ctx))
		})
	}
	if !common.MetricsDisableMemory {
		wg.Go(func() {
			errs.Add(s.collectMemoryInfo(ctx))
		})
	}
	if !common.MetricsDisableDisk {
		wg.Go(func() {
			errs.Add(s.collectDisksInfo(ctx, lastResult))
		})
	}
	if !common.MetricsDisableNetwork {
		wg.Go(func() {
			errs.Add(s.collectNetworkInfo(ctx, lastResult))
		})
	}
	if !common.MetricsDisableSensors {
		wg.Go(func() {
			errs.Add(s.collectSensorsInfo(ctx))
		})
	}
	wg.Wait()

	if errs.HasError() {
		allWarnings := gperr.NewBuilder("")
		allErrors := gperr.NewBuilder("failed to get system info")
		errs.ForEach(func(err error) {
			warnings := new(warning.Warning)
			if errors.As(err, &warnings) {
				for _, warning := range warnings.List {
					if isNoDataAvailable(warning) {
						continue
					}
					allWarnings.Add(warning)
				}
			} else {
				allErrors.Add(err)
			}
		})
		if allWarnings.HasError() {
			log.Warn().Msg(allWarnings.String())
		}
		if allErrors.HasError() {
			return nil, allErrors.Error()
		}
	}

	return &s, nil
}

func (s *SystemInfo) collectCPUInfo(ctx context.Context) error {
	cpuAverage, err := cpu.PercentWithContext(ctx, 500*time.Millisecond, false)
	if err != nil {
		return err
	}
	s.CPUAverage = new(float64)
	*s.CPUAverage = cpuAverage[0]
	return nil
}

func (s *SystemInfo) collectMemoryInfo(ctx context.Context) error {
	memoryInfo, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return err
	}
	s.Memory = memoryInfo
	return nil
}

func (s *SystemInfo) collectDisksInfo(ctx context.Context, lastResult *SystemInfo) error {
	ioCounters, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return err
	}
	s.DisksIO = ioCounters
	if lastResult != nil {
		interval := since(lastResult.Timestamp)
		for name, disk := range s.DisksIO {
			if lastUsage, ok := lastResult.DisksIO[name]; ok {
				disk.ReadSpeed = float64(disk.ReadBytes-lastUsage.ReadBytes) / float64(interval)
				disk.WriteSpeed = float64(disk.WriteBytes-lastUsage.WriteBytes) / float64(interval)
				disk.Iops = diff(disk.ReadCount+disk.WriteCount, lastUsage.ReadCount+lastUsage.WriteCount) / uint64(interval) //nolint:gosec
			}
		}
	}

	partitions, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return err
	}
	s.Disks = make(map[string]*disk.UsageStat, len(partitions))
	errs := gperr.NewBuilder("failed to get disks info")
	for _, partition := range partitions {
		diskInfo, err := disk.UsageWithContext(ctx, partition.Mountpoint)
		if err != nil {
			errs.Add(err)
			continue
		}
		s.Disks[partition.Device] = diskInfo
	}

	if errs.HasError() {
		if len(s.Disks) == 0 {
			return errs.Error()
		}
		log.Warn().Msg(errs.String())
	}
	return nil
}

func (s *SystemInfo) collectNetworkInfo(ctx context.Context, lastResult *SystemInfo) error {
	networkIO, err := net.IOCountersWithContext(ctx, false)
	if err != nil {
		return err
	}
	s.Network = networkIO[0]
	if lastResult != nil {
		interval := float64(since(lastResult.Timestamp))
		s.Network.UploadSpeed = float64(networkIO[0].BytesSent-lastResult.Network.BytesSent) / interval
		s.Network.DownloadSpeed = float64(networkIO[0].BytesRecv-lastResult.Network.BytesRecv) / interval
	}
	return nil
}

func (s *SystemInfo) collectSensorsInfo(ctx context.Context) error {
	sensorsInfo, err := sensors.TemperaturesWithContext(ctx)
	if err != nil {
		return err
	}
	s.Sensors = sensorsInfo
	return nil
}

// recharts friendly.
func aggregate(entries []*SystemInfo, query url.Values) (total int, result Aggregated) {
	n := len(entries)
	aggregated := Aggregated{
		Entries: make([]map[string]any, n),
		Mode:    SystemInfoAggregateMode(query.Get("aggregate")),
	}
	switch aggregated.Mode {
	case SystemInfoAggregateModeCPUAverage:
		for i, entry := range entries {
			if entry.CPUAverage != nil {
				aggregated.Entries[i] = map[string]any{
					"timestamp":   entry.Timestamp,
					"cpu_average": *entry.CPUAverage,
				}
			}
		}
	case SystemInfoAggregateModeMemoryUsage:
		for i, entry := range entries {
			if entry.Memory != nil {
				aggregated.Entries[i] = map[string]any{
					"timestamp":    entry.Timestamp,
					"memory_usage": entry.Memory.Used,
				}
			}
		}
	case SystemInfoAggregateModeMemoryUsagePercent:
		for i, entry := range entries {
			if entry.Memory != nil {
				aggregated.Entries[i] = map[string]any{
					"timestamp":            entry.Timestamp,
					"memory_usage_percent": entry.Memory.UsedPercent,
				}
			}
		}
	case SystemInfoAggregateModeDisksReadSpeed:
		for i, entry := range entries {
			if entry.DisksIO == nil {
				continue
			}
			m := make(map[string]any, len(entry.DisksIO)+1)
			for name, usage := range entry.DisksIO {
				m[name] = usage.ReadSpeed
			}
			m["timestamp"] = entry.Timestamp
			aggregated.Entries[i] = m
		}
	case SystemInfoAggregateModeDisksWriteSpeed:
		for i, entry := range entries {
			if entry.DisksIO == nil {
				continue
			}
			m := make(map[string]any, len(entry.DisksIO)+1)
			for name, usage := range entry.DisksIO {
				m[name] = usage.WriteSpeed
			}
			m["timestamp"] = entry.Timestamp
			aggregated.Entries[i] = m
		}
	case SystemInfoAggregateModeDisksIOPS:
		for i, entry := range entries {
			if entry.DisksIO == nil {
				continue
			}
			m := make(map[string]any, len(entry.DisksIO)+1)
			for name, usage := range entry.DisksIO {
				m[name] = usage.Iops
			}
			m["timestamp"] = entry.Timestamp
			aggregated.Entries[i] = m
		}
	case SystemInfoAggregateModeDiskUsage:
		for i, entry := range entries {
			if entry.Disks == nil {
				continue
			}
			m := make(map[string]any, len(entry.Disks)+1)
			for name, disk := range entry.Disks {
				m[name] = disk.Used
			}
			m["timestamp"] = entry.Timestamp
			aggregated.Entries[i] = m
		}
	case SystemInfoAggregateModeNetworkSpeed:
		for i, entry := range entries {
			if entry.Network == nil {
				continue
			}
			aggregated.Entries[i] = map[string]any{
				"timestamp": entry.Timestamp,
				"upload":    entry.Network.UploadSpeed,
				"download":  entry.Network.DownloadSpeed,
			}
		}
	case SystemInfoAggregateModeNetworkTransfer:
		for i, entry := range entries {
			if entry.Network == nil {
				continue
			}
			aggregated.Entries[i] = map[string]any{
				"timestamp": entry.Timestamp,
				"upload":    entry.Network.BytesSent,
				"download":  entry.Network.BytesRecv,
			}
		}
	case SystemInfoAggregateModeSensorTemperature:
		for i, entry := range entries {
			if entry.Sensors == nil {
				continue
			}
			m := make(map[string]any, len(entry.Sensors)+1)
			for _, sensor := range entry.Sensors {
				m[sensor.SensorKey] = sensor.Temperature
			}
			m["timestamp"] = entry.Timestamp
			aggregated.Entries[i] = m
		}
	default:
		return -1, Aggregated{}
	}
	return len(aggregated.Entries), aggregated
}

func diff(x, y uint64) uint64 {
	if x > y {
		return x - y
	}
	return y - x
}

func since(last int64) int64 {
	now := time.Now().Unix()
	if last > now { // should not happen but just in case
		return 1
	}
	if last == now { // two consecutive polls occur within the same second
		return 1
	}
	return now - last
}

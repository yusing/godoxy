package proxmox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type NodeStats struct {
	KernelVersion string `json:"kernel_version"`
	PVEVersion    string `json:"pve_version"`
	CPUUsage      string `json:"cpu_usage"`
	CPUModel      string `json:"cpu_model"`
	MemUsage      string `json:"mem_usage"`
	MemTotal      string `json:"mem_total"`
	MemPct        string `json:"mem_pct"`
	RootFSUsage   string `json:"rootfs_usage"`
	RootFSTotal   string `json:"rootfs_total"`
	RootFSPct     string `json:"rootfs_pct"`
	Uptime        string `json:"uptime"`
	LoadAvg1m     string `json:"load_avg_1m"`
	LoadAvg5m     string `json:"load_avg_5m"`
	LoadAvg15m    string `json:"load_avg_15m"`
}

// NodeStats streams node stats, like docker stats.
func (n *Node) NodeStats(ctx context.Context, stream bool) (io.ReadCloser, error) {
	if !stream {
		var buf bytes.Buffer
		if err := n.writeNodeStatsLine(ctx, &buf); err != nil {
			return nil, err
		}
		return io.NopCloser(&buf), nil
	}

	pr, pw := io.Pipe()

	go func() {
		writeSample := func() error {
			return n.writeNodeStatsLine(ctx, pw)
		}

		// Match `watch` behavior: write immediately, then on each tick.
		if err := writeSample(); err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		ticker := time.NewTicker(NodeStatsPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = pw.CloseWithError(ctx.Err())
				return
			case <-ticker.C:
				if err := writeSample(); err != nil {
					_ = pw.CloseWithError(err)
					return
				}
			}
		}
	}()

	return pr, nil
}

func (n *Node) writeNodeStatsLine(ctx context.Context, w io.Writer) error {
	// Fetch node status for CPU and memory metrics.
	node, err := n.client.Node(ctx, n.name)
	if err != nil {
		return err
	}

	cpu := fmt.Sprintf("%.1f%%", node.CPU*100)

	memUsage := formatIECBytes(node.Memory.Used)
	memTotal := formatIECBytes(node.Memory.Total)
	memPct := "0.00%"
	if node.Memory.Total > 0 {
		memPct = fmt.Sprintf("%.2f%%", float64(node.Memory.Used)/float64(node.Memory.Total)*100)
	}

	rootFSUsage := formatIECBytes(node.RootFS.Used)
	rootFSTotal := formatIECBytes(node.RootFS.Total)
	rootFSPct := "0.00%"
	if node.RootFS.Total > 0 {
		rootFSPct = fmt.Sprintf("%.2f%%", float64(node.RootFS.Used)/float64(node.RootFS.Total)*100)
	}

	uptime := formatDuration(node.Uptime)

	if len(node.LoadAvg) != 3 {
		return fmt.Errorf("unexpected load average length: %d, expected 3 (1m, 5m, 15m)", len(node.LoadAvg))
	}

	// Linux 6.17.4-1-pve #1 SMP PREEMPT_DYNAMIC PMX 6.17.4-1 (2025-12-03T15:42Z)
	// => 6.17.4-1-pve #1 SMP PREEMPT_DYNAMIC PMX 6.17.4-1 (2025-12-03T15:42Z)
	kversion, _ := strings.CutPrefix(node.Kversion, "Linux ")
	// => 6.17.4-1-pve
	kversion, _, _ = strings.Cut(kversion, " ")

	nodeStats := NodeStats{
		KernelVersion: kversion,
		PVEVersion:    node.PVEVersion,
		CPUUsage:      cpu,
		CPUModel:      node.CPUInfo.Model,
		MemUsage:      memUsage,
		MemTotal:      memTotal,
		MemPct:        memPct,
		RootFSUsage:   rootFSUsage,
		RootFSTotal:   rootFSTotal,
		RootFSPct:     rootFSPct,
		Uptime:        uptime,
		LoadAvg1m:     node.LoadAvg[0],
		LoadAvg5m:     node.LoadAvg[1],
		LoadAvg15m:    node.LoadAvg[2],
	}

	err = json.NewEncoder(w).Encode(nodeStats)
	return err
}

// formatDuration formats uptime in seconds to a human-readable string.
func formatDuration(seconds uint64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

package proxmox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"
)

// const statsScriptLocation = "/tmp/godoxy-stats.sh"

// const statsScript = `#!/bin/sh

// # LXCStats script, written by godoxy.
// printf "%s|%s|%s|%s|%s\n" \
// "$(top -bn1 | grep "Cpu(s)" | sed "s/.*, *\([0-9.]*\)%* id.*/\1/" | awk '{print 100 - $1"%"}')" \
// "$(free -b | awk 'NR==2{printf "%.0f\n%.0f", $3, $2}' | numfmt --to=iec-i --suffix=B | paste -sd/)" \
// "$(free | awk 'NR==2{printf "%.2f%%", $3/$2*100}')" \
// "$(awk 'NR>2{r+=$2;t+=$10}END{printf "%.0f\n%.0f", r, t}' /proc/net/dev | numfmt --to=iec-i --suffix=B | paste -sd/)" \
// "$(awk '{r+=$6;w+=$10}END{printf "%.0f\n%.0f", r*512, w*512}' /proc/diskstats | numfmt --to=iec-i --suffix=B | paste -sd/)"`

// var statsScriptBase64 = base64.StdEncoding.EncodeToString([]byte(statsScript))

// var statsInitCommand = fmt.Sprintf("sh -c 'echo %s | base64 -d > %s && chmod +x %s'", statsScriptBase64, statsScriptLocation, statsScriptLocation)

// var statsStreamScript = fmt.Sprintf("watch -t -w -p -n1 '%s'", statsScriptLocation)
// var statsNonStreamScript = statsScriptLocation

// lxcStatsScriptInit initializes the stats script for the given container.
// func (n *Node) lxcStatsScriptInit(ctx context.Context, vmid int) error {
// 	reader, err := n.LXCCommand(ctx, vmid, statsInitCommand)
// 	if err != nil {
// 		return fmt.Errorf("failed to execute stats init command: %w", err)
// 	}
// 	reader.Close()
// 	return nil
// }

// LXCStats streams container stats, like docker stats.
//
//   - format: "STATUS|CPU%%|MEM USAGE/LIMIT|MEM%%|NET I/O|BLOCK I/O"
//   - example: running|31.1%|9.6GiB/20GiB|48.87%|4.7GiB/3.3GiB|25GiB/36GiB
func (n *Node) LXCStats(ctx context.Context, vmid int, stream bool) (io.ReadCloser, error) {
	if !stream {
		resource, err := n.client.GetResource("lxc", vmid)
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err := writeLXCStatsLine(resource, &buf); err != nil {
			return nil, err
		}
		return io.NopCloser(&buf), nil
	}

	// Validate the resource exists before returning a stream.
	_, err := n.client.GetResource("lxc", vmid)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()

	interval := ResourcePollInterval
	if interval <= 0 {
		interval = time.Second
	}

	go func() {
		writeSample := func() error {
			resource, err := n.client.GetResource("lxc", vmid)
			if err != nil {
				return err
			}
			err = writeLXCStatsLine(resource, pw)
			return err
		}

		// Match `watch` behavior: write immediately, then on each tick.
		if err := writeSample(); err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		ticker := time.NewTicker(interval)
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

func writeLXCStatsLine(resource *proxmox.ClusterResource, w io.Writer) error {
	cpu := fmt.Sprintf("%.1f%%", resource.CPU*100)

	memUsage := formatIECBytes(resource.Mem)
	memLimit := formatIECBytes(resource.MaxMem)
	memPct := "0.00%"
	if resource.MaxMem > 0 {
		memPct = fmt.Sprintf("%.2f%%", float64(resource.Mem)/float64(resource.MaxMem)*100)
	}

	netIO := formatIECBytes(resource.NetIn) + "/" + formatIECBytes(resource.NetOut)
	blockIO := formatIECBytes(resource.DiskRead) + "/" + formatIECBytes(resource.DiskWrite)

	// Keep the format consistent with LXCStatsAlt / `statsScript` (newline terminated).
	_, err := fmt.Fprintf(w, "%s|%s|%s/%s|%s|%s|%s\n", resource.Status, cpu, memUsage, memLimit, memPct, netIO, blockIO)
	return err
}

// formatIECBytes formats a byte count using IEC binary prefixes (KiB, MiB, GiB, ...),
// similar to `numfmt --to=iec-i --suffix=B`.
func formatIECBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}

	prefixes := []string{"B", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei"}
	val := float64(b)
	exp := 0
	for val >= unit && exp < len(prefixes)-1 {
		val /= unit
		exp++
	}

	// One decimal, trimming trailing ".0" to keep output compact (e.g. "10GiB").
	s := fmt.Sprintf("%.1f", val)
	s = strings.TrimSuffix(s, ".0")
	if exp == 0 {
		return s + "B"
	}
	return s + prefixes[exp] + "B"
}

// LXCStatsAlt streams container stats, like docker stats.
//
//   - format: "CPU%%|MEM USAGE/LIMIT|MEM%%|NET I/O|BLOCK I/O"
//   - example: 31.1%|9.6GiB/20GiB|48.87%|4.7GiB/3.3GiB|25TiB/36TiB
// func (n *Node) LXCStatsAlt(ctx context.Context, vmid int, stream bool) (io.ReadCloser, error) {
// 	// Initialize the stats script if it hasn't been initialized yet.
// 	initScriptErr, _ := n.statsScriptInitErrs.LoadOrCompute(vmid,
// 		func() (newValue error, cancel bool) {
// 			if err := n.lxcStatsScriptInit(ctx, vmid); err != nil {
// 				cancel = errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
// 				return err, cancel
// 			}
// 			return nil, false
// 		})

// 	if initScriptErr != nil {
// 		return nil, initScriptErr
// 	}
// 	if stream {
// 		return n.LXCCommand(ctx, vmid, statsStreamScript)
// 	}
// 	return n.LXCCommand(ctx, vmid, statsNonStreamScript)
// }

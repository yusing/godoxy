package qbittorrent

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
)

const endpointLogs = "/api/v2/log/main"

type LogEntry struct {
	ID        int    `json:"id"`
	Timestamp int    `json:"timestamp"`
	Type      int    `json:"type"`
	Message   string `json:"message"`
}

const (
	LogSeverityNormal = 1 << iota
	LogSeverityInfo
	LogSeverityWarning
	LogSeverityCritical
)

func (l *LogEntry) Time() time.Time {
	return time.Unix(int64(l.Timestamp), 0)
}

func (l *LogEntry) Level() string {
	switch l.Type {
	case LogSeverityNormal:
		return "Normal"
	case LogSeverityInfo:
		return "Info"
	case LogSeverityWarning:
		return "Warning"
	case LogSeverityCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

func (l *LogEntry) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"id":        l.ID,
		"timestamp": l.Timestamp,
		"level":     l.Level(),
		"message":   l.Message,
	})
}

// params:
//
//	normal: bool
//	info: bool
//	warning: bool
//	critical: bool
//	last_known_id: int
func (c *Client) GetLogs(ctx context.Context, lastKnownID int) ([]*LogEntry, error) {
	return jsonRequest[[]*LogEntry](ctx, c, endpointLogs, url.Values{
		"last_known_id": {strconv.Itoa(lastKnownID)},
	})
}

func (c *Client) WatchLogs(ctx context.Context) (<-chan *LogEntry, <-chan error) {
	ch := make(chan *LogEntry)
	errCh := make(chan error)

	lastKnownID := -1

	go func() {
		defer close(ch)
		defer close(errCh)

		for {
			select {
			case <-ctx.Done():
				return
			default:
				logs, err := c.GetLogs(ctx, lastKnownID)
				if err != nil {
					errCh <- err
				}

				if len(logs) == 0 {
					time.Sleep(1 * time.Second)
					continue
				}

				for _, log := range logs {
					ch <- log
				}
				lastKnownID = logs[len(logs)-1].ID
			}
		}
	}()

	return ch, errCh
}

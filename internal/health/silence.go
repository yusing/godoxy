package health

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// SilenceWindow is a daily local-time window ("HH:MM-HH:MM") during which
// service-down notifications are suppressed. It may cross midnight.
type SilenceWindow struct {
	Start time.Duration // offset from local midnight
	End   time.Duration
} //	@name	SilenceWindow

var ErrInvalidSilenceWindow = errors.New("invalid silence window, expected HH:MM-HH:MM")

// Parse implements strutils.Parser.
func (w *SilenceWindow) Parse(v string) error {
	startStr, endStr, ok := strings.Cut(strings.TrimSpace(v), "-")
	if !ok {
		return fmt.Errorf("%w: %q", ErrInvalidSilenceWindow, v)
	}
	start, errStart := parseClock(startStr)
	end, errEnd := parseClock(endStr)
	if errStart != nil || errEnd != nil {
		return fmt.Errorf("%w: %q", ErrInvalidSilenceWindow, v)
	}
	if start == end {
		return fmt.Errorf("%w: %q has the same start and end", ErrInvalidSilenceWindow, v)
	}
	w.Start, w.End = start, end
	return nil
}

func parseClock(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if len(s) != len("15:04") {
		return 0, ErrInvalidSilenceWindow
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, err
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}

func (w SilenceWindow) String() string {
	return formatClock(w.Start) + "-" + formatClock(w.End)
}

func formatClock(d time.Duration) string {
	return fmt.Sprintf("%02d:%02d", int(d.Hours()), int(d.Minutes())%60)
}

func (w SilenceWindow) MarshalText() ([]byte, error) {
	return []byte(w.String()), nil
}

func (w *SilenceWindow) UnmarshalText(text []byte) error {
	return w.Parse(string(text))
}

// Contains reports whether t's local wall-clock time falls inside the window.
func (w SilenceWindow) Contains(t time.Time) bool {
	h, m, s := t.Clock()
	now := time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
	if w.Start < w.End {
		return now >= w.Start && now < w.End
	}
	return now >= w.Start || now < w.End
}

// IsSilenced reports whether t falls inside any of the configured silence windows.
func (hc *HealthCheckConfig) IsSilenced(t time.Time) bool {
	for _, w := range hc.Silence {
		if w.Contains(t) {
			return true
		}
	}
	return false
}

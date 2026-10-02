package logadapter

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/goutils/logging"
)

func TestAdapter(t *testing.T) {
	previous, previousLevel := log.Logger, zerolog.GlobalLevel()
	t.Cleanup(func() {
		logging.SetLogger(nil)
		log.Logger = previous
		zerolog.SetGlobalLevel(previousLevel)
	})
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	Register()
	for _, tc := range []struct {
		level logging.Level
		want  string
	}{{logging.Debug, "debug"}, {logging.Info, "info"}, {logging.Warn, "warn"}, {logging.Error, "error"}} {
		t.Run(tc.want, func(t *testing.T) {
			var out bytes.Buffer
			// Registration follows the current logger, rather than capturing it.
			log.Logger = zerolog.New(&out)
			logging.Log(tc.level, "diagnostic", logging.Field{Key: "error", Value: errors.New("failed")}, logging.Field{Key: "count", Value: uint64(42)})
			var record struct {
				Level   string `json:"level"`
				Message string `json:"message"`
				Error   string `json:"error"`
				Count   uint64 `json:"count"`
			}
			if err := json.Unmarshal(out.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if record.Level != tc.want || record.Message != "diagnostic" || record.Error != "failed" || record.Count != 42 {
				t.Fatalf("unexpected record: %+v", record)
			}
		})
	}
	var out bytes.Buffer
	log.Logger = zerolog.New(&out).Level(zerolog.WarnLevel)
	logging.Log(logging.Info, "filtered")
	logging.Log(logging.Level(255), "unknown")
	if out.Len() != 0 {
		t.Fatalf("filtered logs emitted: %s", &out)
	}
	zerolog.SetGlobalLevel(zerolog.Disabled)
	logging.Log(logging.Error, "disabled")
	if out.Len() != 0 {
		t.Fatalf("disabled logs emitted: %s", &out)
	}
}

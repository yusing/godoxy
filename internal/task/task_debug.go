//go:build debug

package task

import (
	"runtime/debug"

	"github.com/rs/zerolog/log"
)

func panicWithDebugStack() {
	panic(string(debug.Stack()))
}

func logStarted(t *Task) {
	log.Info().Msg("task " + t.String() + " started")
}

func logFinished(t *Task) {
	log.Info().Msg("task " + t.String() + " finished")
}

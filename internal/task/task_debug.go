//go:build debug

package task

import (
	"runtime/debug"

	"github.com/rs/zerolog/log"
)

func panicWithDebugStack() {
	panic(string(debug.Stack()))
}

func panicIfFinished(t *Task, reason string) {
	if t.isFinished() {
		log.Panic().Msg("task " + t.String() + " is finished but " + reason)
	}
}

func logStarted(t *Task) {
	log.Info().Msg("task " + t.String() + " started")
}

func logFinished(t *Task) {
	log.Info().Msg("task " + t.String() + " finished")
}

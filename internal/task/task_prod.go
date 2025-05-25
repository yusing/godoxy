//go:build !debug

package task

func panicWithDebugStack() {
	// do nothing
}

func panicIfFinished(t *Task, reason string) {
	// do nothing
}

func logStarted(t *Task) {
	// do nothing
}

func logFinished(t *Task) {
	// do nothing
}

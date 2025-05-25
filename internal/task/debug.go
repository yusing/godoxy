package task

import (
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/gperr"
)

// debug only.
func (t *Task) listStuckedCallbacks() []string {
	callbacks := make([]string, 0, len(t.callbacks))
	for c := range t.callbacks {
		if !c.done.Load() {
			callbacks = append(callbacks, c.about)
		}
	}
	return callbacks
}

// debug only.
func (t *Task) listStuckedChildren() []string {
	children := make([]string, 0, len(t.children))
	for c := range t.children {
		if c.isFinished() {
			continue
		}
		children = append(children, c.String())
		if len(c.children) > 0 {
			children = append(children, c.listStuckedChildren()...)
		}
	}
	return children
}

func (t *Task) reportStucked() {
	callbacks := t.listStuckedCallbacks()
	children := t.listStuckedChildren()
	fmtOutput := gperr.Multiline().
		Addf("stucked callbacks: %d, stucked children: %d",
			len(callbacks), len(children),
		).
		Addf("callbacks").
		AddLinesString(callbacks...).
		Addf("children").
		AddLinesString(children...)
	log.Warn().Msg(fmtOutput.Error())
}

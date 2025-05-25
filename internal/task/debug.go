package task

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/gperr"
)

// debug only.
func (t *Task) listStuckedCallbacks() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	callbacks := make([]string, 0, len(t.callbacksOnFinish))
	for c := range t.callbacksOnFinish {
		callbacks = append(callbacks, c.about)
	}
	return callbacks
}

// debug only.
func (t *Task) listStuckedChildren() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
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
	if len(callbacks) == 0 && len(children) == 0 {
		return
	}
	fmtOutput := gperr.NewBuilder(fmt.Sprintf("%s stucked callbacks: %d, stucked children: %d", t.String(), len(callbacks), len(children)))
	if len(callbacks) > 0 {
		fmtOutput.Add(gperr.New("callbacks").With(gperr.Multiline().AddLinesString(callbacks...)))
	}
	if len(children) > 0 {
		fmtOutput.Add(gperr.New("children").With(gperr.Multiline().AddLinesString(children...)))
	}
	log.Warn().Msg(fmtOutput.String())
}

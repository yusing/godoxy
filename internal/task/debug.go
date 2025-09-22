package task

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/gperr"
)

// debug only.
func listStuckedCallbacks(t *Task) []string {
	callbacks := make([]string, 0)
	if t.onFinish != nil {
		for c := range t.onFinish.Range {
			callbacks = append(callbacks, c.about)
		}
	}
	if t.onCancel != nil {
		for c := range t.onCancel.Range {
			callbacks = append(callbacks, c.about)
		}
	}
	if t.children != nil {
		for c := range t.children.Range {
			callbacks = append(callbacks, listStuckedCallbacks(c)...)
		}
	}
	return callbacks
}

// debug only.
func listStuckedChildren(t *Task) []string {
	if t.children != nil {
		children := make([]string, 0)
		for c := range t.children.Range {
			children = append(children, c.String())
			children = append(children, listStuckedCallbacks(c)...)
		}
		return children
	}
	return nil
}

func (t *Task) reportStucked() {
	callbacks := listStuckedCallbacks(t)
	children := listStuckedChildren(t)
	if len(callbacks) == 0 && len(children) == 0 {
		return
	}
	fmtOutput := gperr.NewBuilder(fmt.Sprintf("%s stucked callbacks: %d, stucked children: %d", t.String(), len(callbacks), len(children)))
	if len(callbacks) > 0 {
		callbackBuilder := gperr.NewBuilder("callbacks")
		for _, c := range callbacks {
			callbackBuilder.Adds(c)
		}
		fmtOutput.Add(callbackBuilder.Error())
	}
	if len(children) > 0 {
		childrenBuilder := gperr.NewBuilder("children")
		for _, c := range children {
			childrenBuilder.Adds(c)
		}
		fmtOutput.Add(childrenBuilder.Error())
	}
	log.Warn().Msg(fmtOutput.String())
}

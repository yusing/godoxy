package task

import (
	"iter"
	"strconv"
	"strings"
)

// debug only.
func (t *Task) listChildren() []string {
	var children []string
	allTasks.Range(func(child *Task) bool {
		if child.parent == t {
			children = append(children, strings.TrimPrefix(child.name, t.name+"."))
		}
		return true
	})
	return children
}

// debug only.
func (t *Task) listCallbacks() []string {
	var callbacks []string
	t.mu.Lock()
	defer t.mu.Unlock()
	for c := range t.callbacks {
		callbacks = append(callbacks, c.about)
	}
	return callbacks
}

func AllTasks() iter.Seq2[string, *Task] {
	return func(yield func(k string, v *Task) bool) {
		for t := range allTasks.Range {
			if !yield(t.name, t) {
				return
			}
		}
	}
}

func (t *Task) Key() string {
	return t.name
}

func (t *Task) callbackList() []map[string]any {
	list := make([]map[string]any, 0, len(t.callbacks))
	for cb := range t.callbacks {
		list = append(list, map[string]any{
			"about":         cb.about,
			"wait_children": strconv.FormatBool(cb.waitChildren),
		})
	}
	return list
}

func (t *Task) MarshalMap() map[string]any {
	return map[string]any{
		"name":          t.name,
		"need_finish":   strconv.FormatBool(t.needFinish),
		"childrens":     t.children,
		"callbacks":     t.callbackList(),
		"finish_called": t.finishedCalled,
	}
}

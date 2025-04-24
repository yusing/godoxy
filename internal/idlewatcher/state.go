package idlewatcher

import idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"

func (w *Watcher) running() bool {
	return w.state.Load().status == idlewatcher.ContainerStatusRunning
}

func (w *Watcher) ready() bool {
	return w.state.Load().ready
}

func (w *Watcher) error() error {
	return w.state.Load().err
}

func (w *Watcher) setReady() {
	w.state.Store(&containerState{
		status: idlewatcher.ContainerStatusRunning,
		ready:  true,
	})
}

func (w *Watcher) setStarting() {
	w.state.Store(&containerState{
		status: idlewatcher.ContainerStatusRunning,
		ready:  false,
	})
}

func (w *Watcher) setNapping(status idlewatcher.ContainerStatus) {
	w.state.Store(&containerState{
		status: status,
		ready:  false,
	})
}

func (w *Watcher) setError(err error) {
	w.state.Store(&containerState{
		status: idlewatcher.ContainerStatusError,
		ready:  false,
		err:    err,
	})
}

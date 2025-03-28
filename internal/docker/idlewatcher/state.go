package idlewatcher

func (w *Watcher) running() bool {
	return w.state.Load().running
}

func (w *Watcher) ready() bool {
	return w.state.Load().ready
}

func (w *Watcher) error() error {
	return w.state.Load().err
}

func (w *Watcher) setReady() {
	w.state.Store(&containerState{
		running: true,
		ready:   true,
	})
}

func (w *Watcher) setStarting() {
	w.state.Store(&containerState{
		running: true,
		ready:   false,
	})
}

func (w *Watcher) setNapping() {
	w.setError(nil)
}

func (w *Watcher) setError(err error) {
	w.state.Store(&containerState{
		running: false,
		ready:   false,
		err:     err,
	})
}

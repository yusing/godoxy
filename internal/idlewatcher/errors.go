package idlewatcher

import (
	"context"
	"errors"
	"fmt"
)

type watcherError struct {
	watcher *Watcher
	err     error
}

func (e *watcherError) Unwrap() error {
	return e.err
}

func (e *watcherError) Error() string {
	return fmt.Sprintf("watcher %q error: %s", e.watcher.cfg.ContainerName(), e.err.Error())
}

func (w *Watcher) newWatcherError(err error) error {
	if errors.Is(err, causeReload) {
		return nil
	}
	if wErr, ok := err.(*watcherError); ok { //nolint:errorlint
		return wErr
	}
	return &watcherError{watcher: w, err: convertError(err)}
}

type depError struct {
	action string
	dep    *dependency
	err    error
}

func (e *depError) Unwrap() error {
	return e.err
}

func (e *depError) Error() string {
	return fmt.Sprintf("%s failed for dependency %q: %s", e.action, e.dep.cfg.ContainerName(), e.err.Error())
}

func (w *Watcher) newDepError(action string, dep *dependency, err error) error {
	if errors.Is(err, causeReload) {
		return nil
	}
	if dErr, ok := err.(*depError); ok { //nolint:errorlint
		return dErr
	}
	return w.newWatcherError(&depError{action: action, dep: dep, err: convertError(err)})
}

func convertError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("timeout")
	default:
		return err
	}
}

package watcher

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/task"
)

func TestDirWatcherDispatchesToFileWatcherWithoutDirectorySubscriber(t *testing.T) {
	const relPath = "config.yml"

	fileEvents := make(chan Event, watcherChannelCapacity+1)
	dirEvents := make(chan Event, watcherChannelCapacity)
	watcherTask := task.GetTestTask(t).Subtask("dir_watcher", true)
	t.Cleanup(func() {
		watcherTask.FinishAndWait(nil)
	})

	w := &DirWatcher{
		Logger:  zerolog.Nop(),
		fwMap:   map[string]*fileWatcher{relPath: &fileWatcher{eventCh: fileEvents}},
		eventCh: dirEvents,
		task:    watcherTask,
	}

	event := Event{
		Type:      watcherEvents.EventTypeFile,
		Action:    watcherEvents.ActionFileWritten,
		ActorName: relPath,
	}

	for range watcherChannelCapacity + 1 {
		require.True(t, w.dispatchEvent(relPath, event))
	}

	require.Empty(t, dirEvents)
	for range watcherChannelCapacity + 1 {
		require.Equal(t, event, <-fileEvents)
	}
}

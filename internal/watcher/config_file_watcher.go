package watcher

import (
	"sync"

	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/goutils/task"
)

var (
	configDirWatcher         *DirWatcher
	configDirWatcherInitOnce sync.Once
)

func initConfigDirWatcher() {
	t := task.RootTask("config_dir_watcher", false)
	configDirWatcher = NewDirectoryWatcher(t, common.ConfigBasePath)
}

// create a new file watcher for file under ConfigBasePath.
func NewConfigFileWatcher(filename string) Watcher {
	configDirWatcherInitOnce.Do(initConfigDirWatcher)
	return configDirWatcher.Add(filename)
}

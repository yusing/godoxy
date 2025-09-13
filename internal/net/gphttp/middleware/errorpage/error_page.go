package errorpage

import (
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/watcher"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

const errPagesBasePath = common.ErrorPagesBasePath

var (
	setupOnce      sync.Once
	dirWatcher     watcher.Watcher
	fileContentMap = xsync.NewMap[string, []byte](xsync.WithGrowOnly())
)

func setup() {
	t := task.RootTask("error_page", false)
	dirWatcher = watcher.NewDirectoryWatcher(t, errPagesBasePath)
	loadContent()
	go watchDir()
}

func GetStaticFile(filename string) ([]byte, bool) {
	setupOnce.Do(setup)
	return fileContentMap.Load(filename)
}

// try <statusCode>.html -> 404.html -> not ok.
func GetErrorPageByStatus(statusCode int) (content []byte, ok bool) {
	content, ok = GetStaticFile(fmt.Sprintf("%d.html", statusCode))
	if !ok && statusCode != 404 {
		return fileContentMap.Load("404.html")
	}
	return
}

func loadContent() {
	files, err := utils.ListFiles(errPagesBasePath, 0)
	if err != nil {
		log.Err(err).Msg("failed to list error page resources")
		return
	}
	for _, file := range files {
		if _, ok := fileContentMap.Load(file); ok {
			continue
		}
		content, err := os.ReadFile(file)
		if err != nil {
			log.Warn().Err(err).Msgf("failed to read error page resource %s", file)
			continue
		}
		file = path.Base(file)
		log.Info().Msgf("error page resource %s loaded", file)
		fileContentMap.Store(file, content)
	}
}

func watchDir() {
	eventCh, errCh := dirWatcher.Events(task.RootContext())
	for {
		select {
		case <-task.RootContextCanceled():
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			filename := event.ActorName
			switch event.Action {
			case events.ActionFileWritten:
				fileContentMap.Delete(filename)
				loadContent()
			case events.ActionFileDeleted:
				fileContentMap.Delete(filename)
				log.Warn().Msgf("error page resource %s deleted", filename)
			case events.ActionFileRenamed:
				log.Warn().Msgf("error page resource %s deleted", filename)
				fileContentMap.Delete(filename)
				loadContent()
			}
		case err := <-errCh:
			gperr.LogError("error watching error page directory", err)
		}
	}
}

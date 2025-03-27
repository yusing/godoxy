package errorpage

import (
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	U "github.com/yusing/go-proxy/internal/utils"
	F "github.com/yusing/go-proxy/internal/utils/functional"
	W "github.com/yusing/go-proxy/internal/watcher"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

const errPagesBasePath = common.ErrorPagesBasePath

var (
	setupOnce      sync.Once
	dirWatcher     W.Watcher
	fileContentMap = F.NewMapOf[string, []byte]()
)

func setup() {
	t := task.RootTask("error_page", false)
	dirWatcher = W.NewDirectoryWatcher(t, errPagesBasePath)
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
	files, err := U.ListFiles(errPagesBasePath, 0)
	if err != nil {
		logging.Err(err).Msg("failed to list error page resources")
		return
	}
	for _, file := range files {
		if fileContentMap.Has(file) {
			continue
		}
		content, err := os.ReadFile(file)
		if err != nil {
			logging.Warn().Err(err).Msgf("failed to read error page resource %s", file)
			continue
		}
		file = path.Base(file)
		logging.Info().Msgf("error page resource %s loaded", file)
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
				logging.Warn().Msgf("error page resource %s deleted", filename)
			case events.ActionFileRenamed:
				logging.Warn().Msgf("error page resource %s deleted", filename)
				fileContentMap.Delete(filename)
				loadContent()
			}
		case err := <-errCh:
			gperr.LogError("error watching error page directory", err)
		}
	}
}

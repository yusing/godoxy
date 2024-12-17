package provider

import (
	"os"
	"path"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/common"
	E "github.com/yusing/go-proxy/internal/error"
	"github.com/yusing/go-proxy/internal/route"
	"github.com/yusing/go-proxy/internal/utils"
	W "github.com/yusing/go-proxy/internal/watcher"
)

type FileProvider struct {
	fileName string
	path     string
	l        zerolog.Logger
}

func FileProviderImpl(filename string) (ProviderImpl, error) {
	impl := &FileProvider{
		fileName: filename,
		path:     path.Join(common.ConfigBasePath, filename),
		l:        logger.With().Str("type", "file").Str("name", filename).Logger(),
	}
	_, err := os.Stat(impl.path)
	if err != nil {
		return nil, err
	}
	return impl, nil
}

func Validate(data []byte) (err E.Error) {
	_, err = utils.DeserializeYAMLMap[*route.RawEntry](data)
	return
}

func (p *FileProvider) String() string {
	return p.fileName
}

func (p *FileProvider) Logger() *zerolog.Logger {
	return &p.l
}

func (p *FileProvider) loadRoutesImpl() (route.Routes, E.Error) {
	routes := route.NewRoutes()

	data, err := os.ReadFile(p.path)
	if err != nil {
		return routes, E.From(err)
	}

	entries, err := utils.DeserializeYAMLMap[*route.RawEntry](data)
	if err == nil {
		return route.FromEntries(entries)
	}
	return routes, E.From(err)
}

func (p *FileProvider) NewWatcher() W.Watcher {
	return W.NewConfigFileWatcher(p.fileName)
}

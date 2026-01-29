package provider

import (
	"os"
	"path"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/serialization"
	W "github.com/yusing/godoxy/internal/watcher"
	gperr "github.com/yusing/goutils/errs"
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
		l:        log.With().Str("type", "file").Str("name", filename).Logger(),
	}
	_, err := os.Stat(impl.path)
	if err != nil {
		return nil, err
	}
	return impl, nil
}

func removeXPrefix(m map[string]any) gperr.Error {
	for alias := range m {
		if strings.HasPrefix(alias, "x-") {
			delete(m, alias)
		}
	}
	return nil
}

func validate(data []byte) (routes route.Routes, err gperr.Error) {
	err = serialization.UnmarshalValidate(data, &routes, yaml.Unmarshal, removeXPrefix)
	return routes, err
}

func Validate(data []byte) (err gperr.Error) {
	_, err = validate(data)
	return err
}

func (p *FileProvider) String() string {
	return p.fileName
}

func (p *FileProvider) ShortName() string {
	return strings.Split(p.fileName, ".")[0]
}

func (p *FileProvider) IsExplicitOnly() bool {
	return false
}

func (p *FileProvider) Logger() *zerolog.Logger {
	return &p.l
}

func (p *FileProvider) loadRoutesImpl() (route.Routes, gperr.Error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return nil, gperr.Wrap(err)
	}
	routes, err := validate(data)
	if err != nil && len(routes) == 0 {
		return nil, gperr.Wrap(err)
	}
	return routes, gperr.Wrap(err)
}

func (p *FileProvider) NewWatcher() W.Watcher {
	return W.NewConfigFileWatcher(p.fileName)
}

package provider

import (
	"os"
	"path"
	"strings"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/route"
	"github.com/yusing/go-proxy/internal/utils"
	W "github.com/yusing/go-proxy/internal/watcher"
	"github.com/yusing/go-proxy/pkg/gperr"
)

type FileProvider struct {
	fileName string
	path     string
	l        zerolog.Logger
}

func FileProviderImpl(filename string) ProviderImpl {
	return &FileProvider{
		fileName: filename,
		path:     path.Join(common.ConfigDir, filename),
		l:        logging.With().Str("type", "file").Str("name", filename).Logger(),
	}
}

func validate(data []byte) (routes route.Routes, err gperr.Error) {
	err = utils.UnmarshalValidateYAML(data, &routes)
	return
}

func Validate(data []byte) (err gperr.Error) {
	_, err = validate(data)
	return
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

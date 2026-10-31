package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routevalidate"
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/godoxy/internal/watcher"
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

func removeXPrefix(m map[string]any) error {
	for alias := range m {
		if strings.HasPrefix(alias, "x-") {
			delete(m, alias)
		}
	}
	return nil
}

func validate(data []byte) (routes route.Routes, err error) {
	err = serialization.UnmarshalValidate(data, &routes, yaml.Unmarshal, removeXPrefix)
	return routes, err
}

// sourceFingerprints returns, per alias, a hash of the route's raw definition
// (anchors and aliases resolved, map keys sorted), so a reload can tell which
// routes actually changed. It returns nil if data cannot be decoded.
func sourceFingerprints(data []byte) map[string]string {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil
	}
	fps := make(map[string]string, len(raw))
	for alias, def := range raw {
		b, err := json.Marshal(def)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(b)
		fps[alias] = hex.EncodeToString(sum[:])
	}
	return fps
}

func Validate(ctx context.Context, data []byte) (err error) {
	routes, err := validate(data)
	var errs gperr.Builder
	errs.Add(err)
	for alias, r := range routes {
		if r == nil {
			continue
		}
		r.Alias = alias
		routevalidate.ResolveProxmox(ctx, r)
		if r.Idlewatcher != nil {
			if err := r.Idlewatcher.ValidateResolved(); err != nil {
				errs.AddSubject(gperr.PrependSubject(err, "idlewatcher"), alias)
			}
		}
	}
	return errs.Error()
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

func (p *FileProvider) loadRoutesImpl(context.Context) (route.Routes, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return nil, err
	}
	routes, err := validate(data)
	if err != nil && len(routes) == 0 {
		return nil, err
	}
	fps := sourceFingerprints(data)
	for alias, r := range routes {
		if r != nil {
			r.SetSourceFingerprint(fps[alias])
		}
	}
	return routes, err
}

func (p *FileProvider) NewWatcher() watcher.Watcher {
	return watcher.NewConfigFileWatcher(p.fileName)
}

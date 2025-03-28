package provider

import (
	"fmt"
	"strconv"

	"github.com/docker/docker/client"
	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/route"
	U "github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"github.com/yusing/go-proxy/internal/watcher"
	"gopkg.in/yaml.v3"
)

type DockerProvider struct {
	name, dockerHost string
	l                zerolog.Logger
}

const (
	aliasRefPrefix    = '#'
	aliasRefPrefixAlt = '$'
)

var ErrAliasRefIndexOutOfRange = gperr.New("index out of range")

func DockerProviderImpl(name, dockerHost string) ProviderImpl {
	if dockerHost == common.DockerHostFromEnv {
		dockerHost = common.GetEnvString("DOCKER_HOST", client.DefaultDockerHost)
	}
	return &DockerProvider{
		name,
		dockerHost,
		logging.With().Str("type", "docker").Str("name", name).Logger(),
	}
}

func (p *DockerProvider) String() string {
	return "docker@" + p.name
}

func (p *DockerProvider) ShortName() string {
	return p.name
}

func (p *DockerProvider) IsExplicitOnly() bool {
	return p.name[len(p.name)-1] == '!'
}

func (p *DockerProvider) Logger() *zerolog.Logger {
	return &p.l
}

func (p *DockerProvider) NewWatcher() watcher.Watcher {
	return watcher.NewDockerWatcher(p.dockerHost)
}

func (p *DockerProvider) loadRoutesImpl() (route.Routes, gperr.Error) {
	containers, err := docker.ListContainers(p.dockerHost)
	if err != nil {
		return nil, gperr.Wrap(err)
	}

	errs := gperr.NewBuilder("")
	routes := make(route.Routes)

	for _, c := range containers {
		container := docker.FromDocker(&c, p.dockerHost)
		if container.IsExcluded {
			continue
		}

		newEntries, err := p.routesFromContainerLabels(container)
		if err != nil {
			errs.Add(err.Subject(container.ContainerName))
		}
		for k, v := range newEntries {
			if routes.Contains(k) {
				errs.Addf("duplicated alias %s", k)
			} else {
				routes[k] = v
			}
		}
	}

	return routes, errs.Error()
}

// Returns a list of proxy entries for a container.
// Always non-nil.
func (p *DockerProvider) routesFromContainerLabels(container *docker.Container) (route.Routes, gperr.Error) {
	if !container.IsExplicit && p.IsExplicitOnly() {
		return nil, nil
	}

	routes := make(route.Routes, len(container.Aliases))

	// init entries map for all aliases
	for _, a := range container.Aliases {
		routes[a] = &route.Route{
			Metadata: route.Metadata{
				Container: container,
			},
		}
	}

	errs := gperr.NewBuilder("label errors")

	m, err := docker.ParseLabels(container.Labels)
	errs.Add(err)

	var wildcardProps docker.LabelMap

	for alias, entryMapAny := range m {
		if len(alias) == 0 {
			errs.Add(gperr.New("empty alias"))
			continue
		}

		entryMap, ok := entryMapAny.(docker.LabelMap)
		if !ok {
			// try to deserialize to map
			entryMap = make(docker.LabelMap)
			yamlStr, ok := entryMapAny.(string)
			if !ok {
				// should not happen
				panic(fmt.Errorf("invalid entry map type %T", entryMapAny))
			}
			if err := yaml.Unmarshal([]byte(yamlStr), &entryMap); err != nil {
				errs.Add(gperr.Wrap(err).Subject(alias))
				continue
			}
		}

		if alias == docker.WildcardAlias {
			wildcardProps = entryMap
			continue
		}

		// check if it is an alias reference
		switch alias[0] {
		case aliasRefPrefix, aliasRefPrefixAlt:
			index, err := strutils.Atoi(alias[1:])
			if err != nil {
				errs.Add(err)
				break
			}
			if index < 1 || index > len(container.Aliases) {
				errs.Add(ErrAliasRefIndexOutOfRange.Subject(strconv.Itoa(index)))
				break
			}
			alias = container.Aliases[index-1]
		}

		// init entry if not exist
		r, ok := routes[alias]
		if !ok {
			r = &route.Route{
				Metadata: route.Metadata{
					Container: container,
				},
			}
			routes[alias] = r
		}

		// deserialize map into entry object
		err := U.Deserialize(entryMap, r)
		if err != nil {
			errs.Add(err.Subject(alias))
		} else {
			routes[alias] = r
		}
	}
	if wildcardProps != nil {
		for _, re := range routes {
			if err := U.Deserialize(wildcardProps, re); err != nil {
				errs.Add(err.Subject(docker.WildcardAlias))
				break
			}
		}
	}

	return routes, errs.Error()
}

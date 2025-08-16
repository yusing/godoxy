package docker

import (
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/yusing/ds/ordered"
	"github.com/yusing/go-proxy/internal/types"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type containerHelper struct {
	*container.Summary
}

// getDeleteLabel gets the value of a label and then deletes it from the container.
// If the label does not exist, an empty string is returned.
func (c containerHelper) getDeleteLabel(label string) string {
	if l, ok := c.Labels[label]; ok {
		delete(c.Labels, label)
		return l
	}
	return ""
}

func (c containerHelper) getAliases() []string {
	if l := c.getDeleteLabel(LabelAliases); l != "" {
		return strutils.CommaSeperatedList(l)
	}
	return []string{c.getName()}
}

func (c containerHelper) getName() string {
	return strings.TrimPrefix(c.Names[0], "/")
}

func (c containerHelper) getMounts() *ordered.Map[string, string] {
	m := ordered.NewMap[string, string](ordered.WithCapacity(len(c.Mounts)))
	for _, v := range c.Mounts {
		m.Set(v.Source, v.Destination)
	}
	return m
}

func (c containerHelper) parseImage() *types.ContainerImage {
	colonSep := strutils.SplitRune(c.Image, ':')
	slashSep := strutils.SplitRune(colonSep[0], '/')
	_, sha256, _ := strings.Cut(c.ImageID, ":")
	im := &types.ContainerImage{
		SHA256:  sha256,
		Version: c.Labels["org.opencontainers.image.version"],
	}
	if len(slashSep) > 1 {
		im.Author = strings.Join(slashSep[:len(slashSep)-1], "/")
		im.Name = slashSep[len(slashSep)-1]
	} else {
		im.Author = "library"
		im.Name = slashSep[0]
	}
	if len(colonSep) > 1 {
		im.Tag = colonSep[1]
	} else {
		im.Tag = "latest"
	}
	return im
}

func (c containerHelper) getPublicPortMapping() types.PortMapping {
	res := make(types.PortMapping)
	for _, v := range c.Ports {
		if v.PublicPort == 0 {
			continue
		}
		res[int(v.PublicPort)] = v
	}
	return res
}

func (c containerHelper) getPrivatePortMapping() types.PortMapping {
	res := make(types.PortMapping)
	for _, v := range c.Ports {
		res[int(v.PrivatePort)] = v
	}
	return res
}

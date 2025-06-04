package docker

import (
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type containerHelper struct {
	*container.SummaryTrimmed
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

func (c containerHelper) getMounts() []string {
	m := make([]string, len(c.Mounts))
	for i, v := range c.Mounts {
		m[i] = v.Destination
	}
	return m
}

func (c containerHelper) parseImage() *ContainerImage {
	colonSep := strutils.SplitRune(c.Image, ':')
	slashSep := strutils.SplitRune(colonSep[0], '/')
	im := new(ContainerImage)
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

func (c containerHelper) getPublicPortMapping() PortMapping {
	res := make(PortMapping)
	for _, v := range c.Ports {
		if v.PublicPort == 0 {
			continue
		}
		res[int(v.PublicPort)] = v
	}
	return res
}

func (c containerHelper) getPrivatePortMapping() PortMapping {
	res := make(PortMapping)
	for _, v := range c.Ports {
		res[int(v.PrivatePort)] = v
	}
	return res
}

package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestContainerExplicit(t *testing.T) {
	tests := []struct {
		name       string
		labels     map[string]string
		isExplicit bool
	}{
		{
			name: "explicit",
			labels: map[string]string{
				"proxy.aliases": "foo",
			},
			isExplicit: true,
		},
		{
			name: "explicit2",
			labels: map[string]string{
				"proxy.idle_timeout": "1s",
			},
			isExplicit: true,
		},
		{
			name:       "not explicit",
			labels:     map[string]string{},
			isExplicit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := FromDocker(&container.SummaryTrimmed{Names: []string{"test"}, State: "test", Labels: tt.labels}, "")
			ExpectEqual(t, c.IsExplicit, tt.isExplicit)
		})
	}
}

func TestContainerHostNetworkMode(t *testing.T) {
	tests := []struct {
		name              string
		container         *container.SummaryTrimmed
		isHostNetworkMode bool
	}{
		{
			name: "host network mode",
			container: &container.SummaryTrimmed{
				Names: []string{"test"},
				State: "test",
				HostConfig: struct {
					NetworkMode string `json:",omitempty"`
				}{
					NetworkMode: "host",
				},
			},
			isHostNetworkMode: true,
		},
		{
			name: "not host network mode",
			container: &container.SummaryTrimmed{
				Names: []string{"test"},
				State: "test",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := FromDocker(tt.container, "")
			ExpectEqual(t, c.IsHostNetworkMode, tt.isHostNetworkMode)
		})
	}
}

func TestImageNameParsing(t *testing.T) {
	tests := []struct {
		full   string
		author string
		image  string
		tag    string
	}{
		{
			full:   "ghcr.io/tensorchord/pgvecto-rs",
			author: "ghcr.io/tensorchord",
			image:  "pgvecto-rs",
			tag:    "latest",
		},
		{
			full:   "redis:latest",
			author: "library",
			image:  "redis",
			tag:    "latest",
		},
		{
			full:   "redis:7.4.0-alpine",
			author: "library",
			image:  "redis",
			tag:    "7.4.0-alpine",
		},
	}
	for _, tt := range tests {
		t.Run(tt.full, func(t *testing.T) {
			helper := containerHelper{&container.SummaryTrimmed{Image: tt.full}}
			im := helper.parseImage()
			ExpectEqual(t, im.Author, tt.author)
			ExpectEqual(t, im.Name, tt.image)
			ExpectEqual(t, im.Tag, tt.tag)
		})
	}
}

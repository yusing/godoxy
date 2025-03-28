package dockerapi

import (
	"context"
	"net/http"
	"sort"

	"github.com/docker/docker/api/types/container"
	"github.com/yusing/go-proxy/internal/gperr"
)

type Container struct {
	Server string `json:"server"`
	Name   string `json:"name"`
	ID     string `json:"id"`
	Image  string `json:"image"`
	State  string `json:"state"`
}

func Containers(w http.ResponseWriter, r *http.Request) {
	serveHTTP[Container, []Container](w, r, GetContainers)
}

func GetContainers(ctx context.Context, dockerClients DockerClients) ([]Container, gperr.Error) {
	errs := gperr.NewBuilder("failed to get containers")
	containers := make([]Container, 0)
	for server, dockerClient := range dockerClients {
		conts, err := dockerClient.ContainerList(ctx, container.ListOptions{All: true})
		if err != nil {
			errs.Add(err)
			continue
		}
		for _, cont := range conts {
			containers = append(containers, Container{
				Server: server,
				Name:   cont.Names[0],
				ID:     cont.ID,
				Image:  cont.Image,
				State:  cont.State,
			})
		}
	}
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})
	if err := errs.Error(); err != nil {
		gperr.LogError("failed to get containers", err)
		if len(containers) == 0 {
			return nil, err
		}
		return containers, nil
	}
	return containers, nil
}

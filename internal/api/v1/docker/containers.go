package dockerapi

import (
	"context"
	"sort"

	"github.com/docker/docker/api/types/container"
	"github.com/gin-gonic/gin"
	"github.com/yusing/go-proxy/internal/gperr"
)

type ContainerState = container.ContainerState // @name ContainerState

type Container struct {
	Server string         `json:"server"`
	Name   string         `json:"name"`
	ID     string         `json:"id"`
	Image  string         `json:"image"`
	State  ContainerState `json:"state,omitempty" extensions:"x-nullable"`
} // @name ContainerResponse

// @x-id				"containers"
// @BasePath		/api/v1
// @Summary		Get containers
// @Description	Get containers
// @Tags			docker
// @Produce		json
// @Success		200	{array}		Container
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/docker/containers [get]
func Containers(c *gin.Context) {
	serveHTTP[Container](c, GetContainers)
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

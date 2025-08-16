package dockerapi

import (
	"context"
	"sort"

	dockerSystem "github.com/docker/docker/api/types/system"
	"github.com/gin-gonic/gin"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type containerStats struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Paused  int `json:"paused"`
	Stopped int `json:"stopped"`
} // @name ContainerStats

type dockerInfo struct {
	Name          string         `json:"name"`
	ServerVersion string         `json:"version"`
	Containers    containerStats `json:"containers"`
	Images        int            `json:"images"`
	NCPU          int            `json:"n_cpu"`
	MemTotal      string         `json:"memory"`
} // @name ServerInfo

func toDockerInfo(info dockerSystem.Info) dockerInfo {
	return dockerInfo{
		Name:          info.Name,
		ServerVersion: info.ServerVersion,
		Containers: containerStats{
			Total:   info.ContainersRunning,
			Running: info.ContainersRunning,
			Paused:  info.ContainersPaused,
			Stopped: info.ContainersStopped,
		},
		Images:   info.Images,
		NCPU:     info.NCPU,
		MemTotal: strutils.FormatByteSize(info.MemTotal),
	}
}

// @x-id				"info"
// @BasePath		/api/v1
// @Summary		Get docker info
// @Description	Get docker info
// @Tags			docker
// @Accept			json
// @Produce		json
// @Success		200	{array}		dockerInfo
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/docker/info [get]
func Info(c *gin.Context) {
	serveHTTP[dockerInfo](c, GetDockerInfo)
}

func GetDockerInfo(ctx context.Context, dockerClients DockerClients) ([]dockerInfo, gperr.Error) {
	errs := gperr.NewBuilder("failed to get docker info")
	dockerInfos := make([]dockerInfo, len(dockerClients))

	i := 0
	for name, dockerClient := range dockerClients {
		info, err := dockerClient.Info(ctx)
		if err != nil {
			errs.Add(err)
			continue
		}
		info.Name = name
		dockerInfos[i] = toDockerInfo(info)
		i++
	}

	sort.Slice(dockerInfos, func(i, j int) bool {
		return dockerInfos[i].Name < dockerInfos[j].Name
	})
	return dockerInfos, errs.Error()
}

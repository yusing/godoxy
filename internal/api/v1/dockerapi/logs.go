package dockerapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/task"
)

// FIXME: agent logs not updating.
func Logs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	server := r.PathValue("server")
	containerID := r.PathValue("container")
	stdout, _ := strconv.ParseBool(query.Get("stdout"))
	stderr, _ := strconv.ParseBool(query.Get("stderr"))
	since := query.Get("from")
	until := query.Get("to")
	levels := query.Get("levels") // TODO: implement levels

	dockerClient, found, err := getDockerClient(server)
	if err != nil {
		gphttp.BadRequest(w, err.Error())
		return
	}
	if !found {
		gphttp.NotFound(w, "server not found")
		return
	}
	defer dockerClient.Close()

	opts := container.LogsOptions{
		ShowStdout: stdout,
		ShowStderr: stderr,
		Since:      since,
		Until:      until,
		Timestamps: true,
		Follow:     true,
		Tail:       "100",
	}
	if levels != "" {
		opts.Details = true
	}

	logs, err := dockerClient.ContainerLogs(r.Context(), containerID, opts)
	if err != nil {
		gphttp.BadRequest(w, err.Error())
		return
	}
	defer logs.Close()

	conn, err := gpwebsocket.Initiate(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	writer := gpwebsocket.NewWriter(r.Context(), conn, websocket.TextMessage)
	_, err = stdcopy.StdCopy(writer, writer, logs) // de-multiplex logs
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, task.ErrProgramExiting) {
			return
		}
		log.Err(err).
			Str("server", server).
			Str("container", containerID).
			Msg("failed to de-multiplex logs")
	}
}

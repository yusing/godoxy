package dockerapi

import (
	"net/http"

	"github.com/coder/websocket"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

func Logs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	server := r.PathValue("server")
	containerID := r.PathValue("container")
	stdout := strutils.ParseBool(query.Get("stdout"))
	stderr := strutils.ParseBool(query.Get("stderr"))
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
	defer conn.CloseNow()

	writer := gpwebsocket.NewWriter(r.Context(), conn, websocket.MessageText)
	_, err = stdcopy.StdCopy(writer, writer, logs) // de-multiplex logs
	if err != nil {
		logging.Err(err).
			Str("server", server).
			Str("container", containerID).
			Msg("failed to de-multiplex logs")
	}
}

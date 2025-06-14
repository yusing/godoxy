package v1

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	_ "embed"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/agent/pkg/certs"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp"
)

func NewAgent(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("name")
	if name == "" {
		gphttp.MissingKey(w, "name")
		return
	}
	host := q.Get("host")
	if host == "" {
		gphttp.MissingKey(w, "host")
		return
	}
	portStr := q.Get("port")
	if portStr == "" {
		gphttp.MissingKey(w, "port")
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		gphttp.InvalidKey(w, "port")
		return
	}
	hostport := fmt.Sprintf("%s:%d", host, port)
	if _, ok := agent.GetAgent(hostport); ok {
		gphttp.KeyAlreadyExists(w, "agent", hostport)
		return
	}
	t := q.Get("type")
	switch t {
	case "docker", "system":
		break
	case "":
		gphttp.MissingKey(w, "type")
		return
	default:
		gphttp.InvalidKey(w, "type")
		return
	}

	nightly, _ := strconv.ParseBool(q.Get("nightly"))
	var image string
	if nightly {
		image = agent.DockerImageNightly
	} else {
		image = agent.DockerImageProduction
	}

	ca, srv, client, err := agent.NewAgent()
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}

	var cfg agent.Generator = &agent.AgentEnvConfig{
		Name:    name,
		Port:    port,
		CACert:  ca.String(),
		SSLCert: srv.String(),
	}
	if t == "docker" {
		cfg = &agent.AgentComposeConfig{
			Image:          image,
			AgentEnvConfig: cfg.(*agent.AgentEnvConfig),
		}
	}
	template, err := cfg.Generate()
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}

	gphttp.RespondJSON(w, r, map[string]any{
		"compose": template,
		"ca":      ca,
		"client":  client,
	})
}

func VerifyNewAgent(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	clientPEMData, err := io.ReadAll(r.Body)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}

	var data struct {
		Host   string        `json:"host"`
		CA     agent.PEMPair `json:"ca"`
		Client agent.PEMPair `json:"client"`
	}

	if err := json.Unmarshal(clientPEMData, &data); err != nil {
		gphttp.ClientError(w, r, err)
		return
	}

	nRoutesAdded, err := config.GetInstance().VerifyNewAgent(data.Host, data.CA, data.Client)
	if err != nil {
		gphttp.ClientError(w, r, err)
		return
	}

	zip, err := certs.ZipCert(data.CA.Cert, data.Client.Cert, data.Client.Key)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}

	filename, ok := certs.AgentCertsFilepath(data.Host)
	if !ok {
		gphttp.InvalidKey(w, "host")
		return
	}

	if err := os.WriteFile(filename, zip, 0600); err != nil {
		gphttp.ServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(fmt.Appendf(nil, "Added %d routes", nRoutesAdded))
}

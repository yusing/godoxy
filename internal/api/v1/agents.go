package v1

import (
	"net/http"
	"time"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
)

func ListAgents(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	gpwebsocket.DynamicJSONHandler(w, r, agent.Agents.Slice, 10*time.Second)
}

package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/agent/pkg/env"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging/memlogger"
	"github.com/yusing/go-proxy/internal/metrics/systeminfo"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"github.com/yusing/go-proxy/pkg"
)

type ServeMux struct{ *http.ServeMux }

func (mux ServeMux) HandleMethods(methods, endpoint string, handler http.HandlerFunc) {
	for _, m := range strutils.CommaSeperatedList(methods) {
		mux.ServeMux.HandleFunc(m+" "+agent.APIEndpointBase+endpoint, handler)
	}
}

func (mux ServeMux) HandleFunc(endpoint string, handler http.HandlerFunc) {
	mux.ServeMux.HandleFunc(agent.APIEndpointBase+endpoint, handler)
}

type NopWriteCloser struct {
	io.Writer
}

func (NopWriteCloser) Close() error {
	return nil
}

func NewAgentHandler() http.Handler {
	mux := ServeMux{http.NewServeMux()}

	mux.HandleFunc(agent.EndpointProxyHTTP+"/{path...}", ProxyHTTP)
	mux.HandleMethods("GET", agent.EndpointVersion, pkg.GetVersionHTTPHandler())
	mux.HandleMethods("GET", agent.EndpointName, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, env.AgentName)
	})
	mux.HandleMethods("GET", agent.EndpointHealth, CheckHealth)
	mux.HandleMethods("GET", agent.EndpointLogs, memlogger.HandlerFunc())
	mux.HandleMethods("GET", agent.EndpointSystemInfo, systeminfo.Poller.ServeHTTP)
	mux.ServeMux.HandleFunc("/", DockerSocketHandler())
	return mux
}

func endpointNotAllowed(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Endpoint not allowed", http.StatusForbidden)
}

// ref: https://github.com/Tecnativa/docker-socket-proxy/blob/master/haproxy.cfg
func NewDockerHandler() http.Handler {
	r := mux.NewRouter()
	var socketHandler http.HandlerFunc
	if common.IsTest {
		socketHandler = mockDockerSocketHandler()
	} else {
		socketHandler = DockerSocketHandler()
	}

	const apiVersionPrefix = `/{version:(?:v[\d\.]+)?}`
	const containerPath = "/containers/{id:[a-zA-Z0-9_.-]+}"

	allowedPaths := []string{}
	deniedPaths := []string{}

	if env.DockerContainers {
		allowedPaths = append(allowedPaths, "/containers")
		if !env.DockerRestarts {
			deniedPaths = append(deniedPaths, containerPath+"/stop")
			deniedPaths = append(deniedPaths, containerPath+"/restart")
			deniedPaths = append(deniedPaths, containerPath+"/kill")
		}
		if !env.DockerStart {
			deniedPaths = append(deniedPaths, containerPath+"/start")
		}
		if !env.DockerStop && env.DockerRestarts {
			deniedPaths = append(deniedPaths, containerPath+"/stop")
		}
	}
	if env.DockerAuth {
		allowedPaths = append(allowedPaths, "/auth")
	}
	if env.DockerBuild {
		allowedPaths = append(allowedPaths, "/build")
	}
	if env.DockerCommit {
		allowedPaths = append(allowedPaths, "/commit")
	}
	if env.DockerConfigs {
		allowedPaths = append(allowedPaths, "/configs")
	}
	if env.DockerDistributions {
		allowedPaths = append(allowedPaths, "/distributions")
	}
	if env.DockerEvents {
		allowedPaths = append(allowedPaths, "/events")
	}
	if env.DockerExec {
		allowedPaths = append(allowedPaths, "/exec")
	}
	if env.DockerGrpc {
		allowedPaths = append(allowedPaths, "/grpc")
	}
	if env.DockerImages {
		allowedPaths = append(allowedPaths, "/images")
	}
	if env.DockerInfo {
		allowedPaths = append(allowedPaths, "/info")
	}
	if env.DockerNetworks {
		allowedPaths = append(allowedPaths, "/networks")
	}
	if env.DockerNodes {
		allowedPaths = append(allowedPaths, "/nodes")
	}
	if env.DockerPing {
		allowedPaths = append(allowedPaths, "/_ping")
	}
	if env.DockerPlugins {
		allowedPaths = append(allowedPaths, "/plugins")
	}
	if env.DockerSecrets {
		allowedPaths = append(allowedPaths, "/secrets")
	}
	if env.DockerServices {
		allowedPaths = append(allowedPaths, "/services")
	}
	if env.DockerSession {
		allowedPaths = append(allowedPaths, "/session")
	}
	if env.DockerSwarm {
		allowedPaths = append(allowedPaths, "/swarm")
	}
	if env.DockerSystem {
		allowedPaths = append(allowedPaths, "/system")
	}
	if env.DockerTasks {
		allowedPaths = append(allowedPaths, "/tasks")
	}
	if env.DockerVersion {
		allowedPaths = append(allowedPaths, "/version")
	}
	if env.DockerVolumes {
		allowedPaths = append(allowedPaths, "/volumes")
	}

	// Helper to determine if a path should be treated as a prefix
	isPrefixPath := func(path string) bool {
		return strings.Count(path, "/") == 1
	}

	// 1. Register Denied Paths (specific)
	for _, path := range deniedPaths {
		// Handle with version prefix
		r.HandleFunc(apiVersionPrefix+path, endpointNotAllowed)
		// Handle without version prefix
		r.HandleFunc(path, endpointNotAllowed)
	}

	// 2. Register Allowed Paths
	for _, p := range allowedPaths {
		fullPathWithVersion := apiVersionPrefix + p
		if isPrefixPath(p) {
			r.PathPrefix(fullPathWithVersion).Handler(socketHandler)
			r.PathPrefix(p).Handler(socketHandler)
		} else {
			r.HandleFunc(fullPathWithVersion, socketHandler)
			r.HandleFunc(p, socketHandler)
		}
	}

	// 3. Add fallback for any other routes
	r.PathPrefix("/").HandlerFunc(endpointNotAllowed)

	// HTTP method filtering
	if !env.DockerPost {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			switch req.Method {
			case http.MethodGet:
				r.ServeHTTP(w, req)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost, http.MethodGet:
			r.ServeHTTP(w, req)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

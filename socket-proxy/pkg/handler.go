package socketproxy

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/yusing/go-proxy/socketproxy/pkg/reverseproxy"
)

var dialer = &net.Dialer{KeepAlive: 1 * time.Second}

func dialDockerSocket(socket string) func(ctx context.Context, _, _ string) (net.Conn, error) {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", socket)
	}
}

var DockerSocketHandler = dockerSocketHandler

func dockerSocketHandler(socket string) http.HandlerFunc {
	rp := &reverseproxy.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "api.moby.localhost"
			req.RequestURI = req.URL.String()
		},
		Transport: &http.Transport{
			DialContext:        dialDockerSocket(socket),
			DisableCompression: true,
		},
	}

	return rp.ServeHTTP
}

func endpointNotAllowed(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Endpoint not allowed", http.StatusForbidden)
}

// ref: https://github.com/Tecnativa/docker-socket-proxy/blob/master/haproxy.cfg
func NewHandler() http.Handler {
	r := mux.NewRouter()
	socketHandler := DockerSocketHandler(DockerSocket)

	const apiVersionPrefix = `/{version:(?:v[\d\.]+)?}`
	const containerPath = "/containers/{id:[a-zA-Z0-9_.-]+}"

	allowedPaths := []string{}
	deniedPaths := []string{}

	if DockerContainers {
		allowedPaths = append(allowedPaths, "/containers")
		if !DockerRestarts {
			deniedPaths = append(deniedPaths, containerPath+"/stop")
			deniedPaths = append(deniedPaths, containerPath+"/restart")
			deniedPaths = append(deniedPaths, containerPath+"/kill")
		}
		if !DockerStart {
			deniedPaths = append(deniedPaths, containerPath+"/start")
		}
		if !DockerStop && DockerRestarts {
			deniedPaths = append(deniedPaths, containerPath+"/stop")
		}
	}
	if DockerAuth {
		allowedPaths = append(allowedPaths, "/auth")
	}
	if DockerBuild {
		allowedPaths = append(allowedPaths, "/build")
	}
	if DockerCommit {
		allowedPaths = append(allowedPaths, "/commit")
	}
	if DockerConfigs {
		allowedPaths = append(allowedPaths, "/configs")
	}
	if DockerDistribution {
		allowedPaths = append(allowedPaths, "/distribution")
	}
	if DockerEvents {
		allowedPaths = append(allowedPaths, "/events")
	}
	if DockerExec {
		allowedPaths = append(allowedPaths, "/exec")
	}
	if DockerGrpc {
		allowedPaths = append(allowedPaths, "/grpc")
	}
	if DockerImages {
		allowedPaths = append(allowedPaths, "/images")
	}
	if DockerInfo {
		allowedPaths = append(allowedPaths, "/info")
	}
	if DockerNetworks {
		allowedPaths = append(allowedPaths, "/networks")
	}
	if DockerNodes {
		allowedPaths = append(allowedPaths, "/nodes")
	}
	if DockerPing {
		allowedPaths = append(allowedPaths, "/_ping")
	}
	if DockerPlugins {
		allowedPaths = append(allowedPaths, "/plugins")
	}
	if DockerSecrets {
		allowedPaths = append(allowedPaths, "/secrets")
	}
	if DockerServices {
		allowedPaths = append(allowedPaths, "/services")
	}
	if DockerSession {
		allowedPaths = append(allowedPaths, "/session")
	}
	if DockerSwarm {
		allowedPaths = append(allowedPaths, "/swarm")
	}
	if DockerSystem {
		allowedPaths = append(allowedPaths, "/system")
	}
	if DockerTasks {
		allowedPaths = append(allowedPaths, "/tasks")
	}
	if DockerVersion {
		allowedPaths = append(allowedPaths, "/version")
	}
	if DockerVolumes {
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
	if !DockerPost {
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

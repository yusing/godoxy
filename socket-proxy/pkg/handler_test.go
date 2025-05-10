package socketproxy_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	. "github.com/yusing/go-proxy/socketproxy/pkg"
)

func mockDockerSocketHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("mock docker response"))
	})
}

func TestMain(m *testing.M) {
	DockerSocketHandler = mockDockerSocketHandler
	os.Exit(m.Run())
}

func TestNewDockerHandler(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		envSetup       func()
		wantStatusCode int
	}{
		{
			name:           "GET _ping allowed by default",
			method:         http.MethodGet,
			path:           "/_ping",
			envSetup:       func() {},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "GET version allowed by default",
			method:         http.MethodGet,
			path:           "/version",
			envSetup:       func() {},
			wantStatusCode: http.StatusOK,
		},
		{
			name:   "GET containers allowed when enabled",
			method: http.MethodGet,
			path:   "/containers",
			envSetup: func() {
				DockerContainers = true
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:   "GET containers not allowed when disabled",
			method: http.MethodGet,
			path:   "/containers",
			envSetup: func() {
				DockerContainers = false
			},
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:   "POST not allowed by default",
			method: http.MethodPost,
			path:   "/_ping",
			envSetup: func() {
				DockerPost = false
			},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:   "POST allowed when enabled",
			method: http.MethodPost,
			path:   "/_ping",
			envSetup: func() {
				DockerPost = true
				DockerPing = true
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:   "Container restart not allowed when disabled",
			method: http.MethodPost,
			path:   "/containers/test-container/restart",
			envSetup: func() {
				DockerPost = true
				DockerContainers = true
				DockerRestarts = false
			},
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:   "Container restart allowed when enabled",
			method: http.MethodPost,
			path:   "/containers/test-container/restart",
			envSetup: func() {
				DockerPost = true
				DockerContainers = true
				DockerRestarts = true
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:   "Container start not allowed when disabled",
			method: http.MethodPost,
			path:   "/containers/test-container/start",
			envSetup: func() {
				DockerPost = true
				DockerContainers = true
				DockerStart = false
			},
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:   "Container start allowed when enabled",
			method: http.MethodPost,
			path:   "/containers/test-container/start",
			envSetup: func() {
				DockerPost = true
				DockerContainers = true
				DockerStart = true
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:   "Container stop not allowed when disabled",
			method: http.MethodPost,
			path:   "/containers/test-container/stop",
			envSetup: func() {
				DockerPost = true
				DockerContainers = true
				DockerStop = false
			},
			wantStatusCode: http.StatusForbidden,
		},
		{
			name:   "Container stop allowed when enabled",
			method: http.MethodPost,
			path:   "/containers/test-container/stop",
			envSetup: func() {
				DockerPost = true
				DockerContainers = true
				DockerStop = true
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:   "Versioned API paths work",
			method: http.MethodGet,
			path:   "/v1.41/version",
			envSetup: func() {
				DockerVersion = true
			},
			wantStatusCode: http.StatusOK,
		},
		{
			name:   "PUT method not allowed",
			method: http.MethodPut,
			path:   "/version",
			envSetup: func() {
				DockerVersion = true
			},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
		{
			name:   "DELETE method not allowed",
			method: http.MethodDelete,
			path:   "/version",
			envSetup: func() {
				DockerVersion = true
			},
			wantStatusCode: http.StatusMethodNotAllowed,
		},
	}

	// Save original env values to restore after tests
	originalContainers := DockerContainers
	originalRestarts := DockerRestarts
	originalStart := DockerStart
	originalStop := DockerStop
	originalPost := DockerPost
	originalPing := DockerPing
	originalVersion := DockerVersion

	defer func() {
		// Restore original values
		DockerContainers = originalContainers
		DockerRestarts = originalRestarts
		DockerStart = originalStart
		DockerStop = originalStop
		DockerPost = originalPost
		DockerPing = originalPing
		DockerVersion = originalVersion
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment for this test
			tt.envSetup()

			// Create test handler that will record the response for verification
			dockerHandler := NewHandler()

			// Test server to capture the response
			recorder := httptest.NewRecorder()

			// Create request
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			// Process the request
			dockerHandler.ServeHTTP(recorder, req)

			// Check response
			if recorder.Code != tt.wantStatusCode {
				t.Errorf("Expected status code %d, got %d",
					tt.wantStatusCode, recorder.Code)
			}
		})
	}
}

// This test focuses on checking that all the path prefix handling works correctly
func TestNewDockerHandler_PathHandling(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		envVarName  string
		envVarValue bool
		method      string
		wantAllowed bool
	}{
		{"Container path", "/containers/json", "DockerContainers", true, http.MethodGet, true},
		{"Container path disabled", "/containers/json", "DockerContainers", false, http.MethodGet, false},

		{"Auth path", "/auth", "DockerAuth", true, http.MethodGet, true},
		{"Auth path disabled", "/auth", "DockerAuth", false, http.MethodGet, false},

		{"Build path", "/build", "DockerBuild", true, http.MethodGet, true},
		{"Build path disabled", "/build", "DockerBuild", false, http.MethodGet, false},

		{"Commit path", "/commit", "DockerCommit", true, http.MethodGet, true},
		{"Commit path disabled", "/commit", "DockerCommit", false, http.MethodGet, false},

		{"Configs path", "/configs", "DockerConfigs", true, http.MethodGet, true},
		{"Configs path disabled", "/configs", "DockerConfigs", false, http.MethodGet, false},

		{"Distribution path", "/distribution", "DockerDistribution", true, http.MethodGet, true},
		{"Distribution path disabled", "/distribution", "DockerDistribution", false, http.MethodGet, false},

		{"Events path", "/events", "DockerEvents", true, http.MethodGet, true},
		{"Events path disabled", "/events", "DockerEvents", false, http.MethodGet, false},

		{"Exec path", "/exec", "DockerExec", true, http.MethodGet, true},
		{"Exec path disabled", "/exec", "DockerExec", false, http.MethodGet, false},

		{"Grpc path", "/grpc", "DockerGrpc", true, http.MethodGet, true},
		{"Grpc path disabled", "/grpc", "DockerGrpc", false, http.MethodGet, false},

		{"Images path", "/images", "DockerImages", true, http.MethodGet, true},
		{"Images path disabled", "/images", "DockerImages", false, http.MethodGet, false},

		{"Info path", "/info", "DockerInfo", true, http.MethodGet, true},
		{"Info path disabled", "/info", "DockerInfo", false, http.MethodGet, false},

		{"Networks path", "/networks", "DockerNetworks", true, http.MethodGet, true},
		{"Networks path disabled", "/networks", "DockerNetworks", false, http.MethodGet, false},

		{"Nodes path", "/nodes", "DockerNodes", true, http.MethodGet, true},
		{"Nodes path disabled", "/nodes", "DockerNodes", false, http.MethodGet, false},

		{"Plugins path", "/plugins", "DockerPlugins", true, http.MethodGet, true},
		{"Plugins path disabled", "/plugins", "DockerPlugins", false, http.MethodGet, false},

		{"Secrets path", "/secrets", "DockerSecrets", true, http.MethodGet, true},
		{"Secrets path disabled", "/secrets", "DockerSecrets", false, http.MethodGet, false},

		{"Services path", "/services", "DockerServices", true, http.MethodGet, true},
		{"Services path disabled", "/services", "DockerServices", false, http.MethodGet, false},

		{"Session path", "/session", "DockerSession", true, http.MethodGet, true},
		{"Session path disabled", "/session", "DockerSession", false, http.MethodGet, false},

		{"Swarm path", "/swarm", "DockerSwarm", true, http.MethodGet, true},
		{"Swarm path disabled", "/swarm", "DockerSwarm", false, http.MethodGet, false},

		{"System path", "/system", "DockerSystem", true, http.MethodGet, true},
		{"System path disabled", "/system", "DockerSystem", false, http.MethodGet, false},

		{"Tasks path", "/tasks", "DockerTasks", true, http.MethodGet, true},
		{"Tasks path disabled", "/tasks", "DockerTasks", false, http.MethodGet, false},

		{"Volumes path", "/volumes", "DockerVolumes", true, http.MethodGet, true},
		{"Volumes path disabled", "/volumes", "DockerVolumes", false, http.MethodGet, false},

		// Test versioned paths
		{"Versioned auth", "/v1.41/auth", "DockerAuth", true, http.MethodGet, true},
		{"Versioned auth disabled", "/v1.41/auth", "DockerAuth", false, http.MethodGet, false},
	}

	defer func() {
		// Restore original env values
		Load()
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset all Docker* env vars to false for this test
			Load()

			// Enable POST for all these tests
			DockerPost = true

			// Set the specific env var for this test
			switch tt.envVarName {
			case "DockerContainers":
				DockerContainers = tt.envVarValue
			case "DockerRestarts":
				DockerRestarts = tt.envVarValue
			case "DockerStart":
				DockerStart = tt.envVarValue
			case "DockerStop":
				DockerStop = tt.envVarValue
			case "DockerAuth":
				DockerAuth = tt.envVarValue
			case "DockerBuild":
				DockerBuild = tt.envVarValue
			case "DockerCommit":
				DockerCommit = tt.envVarValue
			case "DockerConfigs":
				DockerConfigs = tt.envVarValue
			case "DockerDistribution":
				DockerDistribution = tt.envVarValue
			case "DockerEvents":
				DockerEvents = tt.envVarValue
			case "DockerExec":
				DockerExec = tt.envVarValue
			case "DockerGrpc":
				DockerGrpc = tt.envVarValue
			case "DockerImages":
				DockerImages = tt.envVarValue
			case "DockerInfo":
				DockerInfo = tt.envVarValue
			case "DockerNetworks":
				DockerNetworks = tt.envVarValue
			case "DockerNodes":
				DockerNodes = tt.envVarValue
			case "DockerPlugins":
				DockerPlugins = tt.envVarValue
			case "DockerSecrets":
				DockerSecrets = tt.envVarValue
			case "DockerServices":
				DockerServices = tt.envVarValue
			case "DockerSession":
				DockerSession = tt.envVarValue
			case "DockerSwarm":
				DockerSwarm = tt.envVarValue
			case "DockerSystem":
				DockerSystem = tt.envVarValue
			case "DockerTasks":
				DockerTasks = tt.envVarValue
			case "DockerVolumes":
				DockerVolumes = tt.envVarValue
			default:
				t.Fatalf("Unknown env var: %s", tt.envVarName)
			}

			// Create test handler
			dockerHandler := NewHandler()

			// Test server to capture the response
			recorder := httptest.NewRecorder()

			// Create request
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			// Process the request
			dockerHandler.ServeHTTP(recorder, req)

			// Check if the status indicates if the path is allowed or not
			isAllowed := recorder.Code != http.StatusForbidden
			if isAllowed != tt.wantAllowed {
				t.Errorf("Path %s with env %s=%v: got allowed=%v, want allowed=%v (status=%d)",
					tt.path, tt.envVarName, tt.envVarValue, isAllowed, tt.wantAllowed, recorder.Code)
			}
		})
	}
}

// TestNewDockerHandlerWithMockDocker mocks the Docker API to test the actual HTTP handler behavior
// This is a more comprehensive test that verifies the full request/response chain
func TestNewDockerHandlerWithMockDocker(t *testing.T) {
	// Set up environment
	DockerContainers = true
	DockerPost = true

	// Create the handler
	handler := NewHandler()

	// Test a valid request
	req, _ := http.NewRequest(http.MethodGet, "/containers", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status OK for /containers, got %d", recorder.Code)
	}

	// Test a disallowed path
	DockerContainers = false
	handler = NewHandler() // recreate with new env

	req, _ = http.NewRequest(http.MethodGet, "/containers", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("Expected status Forbidden for /containers when disabled, got %d", recorder.Code)
	}
}

package proxmox

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	goproxmox "github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	"github.com/yusing/goutils/task"
)

func TestNodePoolsAreIsolatedByTaskContext(t *testing.T) {
	parent := task.GetTestTask(t)
	activeTask := parent.Subtask("active", false)
	candidateTask := parent.Subtask("candidate", false)
	activeNodes := NewNodePool()
	candidateNodes := NewNodePool()
	SetCtx(activeTask, activeNodes)
	SetCtx(candidateTask, candidateNodes)

	activeNode := NewNode(NewClient("https://active.example/api2/json"), "pve", "node/pve")
	candidateNode := NewNode(NewClient("https://candidate.example/api2/json"), "pve", "node/pve")
	activeNodes.Add(activeNode)
	candidateNodes.Add(candidateNode)

	gotActive, err := NodeFromCtx(activeTask.Context(), "pve")
	require.NoError(t, err)
	require.Same(t, activeNode, gotActive)

	gotCandidate, err := NodeFromCtx(candidateTask.Context(), "pve")
	require.NoError(t, err)
	require.Same(t, candidateNode, gotCandidate)

	emptyTask := parent.Subtask("replacement", false)
	SetCtx(emptyTask, NewNodePool())
	_, err = NodeFromCtx(emptyTask.Context(), "pve")
	require.ErrorIs(t, err, ErrNodeNotFound)
}

func TestNodePoolRejectsAmbiguousNamesWithoutAffectingUnrelatedNodes(t *testing.T) {
	nodes := NewNodePool()
	nodes.Add(NewNode(NewClient("https://one.example/api2/json"), "pve", "node/pve"))
	nodes.Add(NewNode(NewClient("https://two.example/api2/json"), "pve", "node/pve"))
	unique := NewNode(NewClient("https://three.example/api2/json"), "edge", "node/edge")
	nodes.Add(unique)

	_, err := nodes.Get("pve")
	require.ErrorIs(t, err, ErrNodeAmbiguous)
	require.ErrorContains(t, err, "one.example")
	require.ErrorContains(t, err, "two.example")

	got, err := nodes.Get("edge")
	require.NoError(t, err)
	require.Same(t, unique, got)

	_, err = nodes.Get("missing")
	require.ErrorIs(t, err, ErrNodeNotFound)
}

func TestNodePoolReplacesTheSameProvidersNode(t *testing.T) {
	nodes := NewNodePool()
	client := NewClient("https://one.example/api2/json")
	oldNode := NewNode(client, "pve", "node/pve-old")
	newNode := NewNode(client, "pve", "node/pve-new")
	nodes.Add(oldNode)
	nodes.Add(newNode)

	got, err := nodes.Get("pve")
	require.NoError(t, err)
	require.Same(t, newNode, got)
}

func TestNodeFromContextReportsMissingPool(t *testing.T) {
	_, err := NodeFromCtx(t.Context(), "pve")
	require.ErrorIs(t, err, ErrNodePoolUnavailable)
}

func TestUpdateClusterInfoRegistersNodesInTheContextPoolAndClient(t *testing.T) {
	var clusterStatusCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version":
			_, _ = w.Write([]byte(`{"data":{"repoid":"test","release":"9.1","version":"9.1-1"}}`))
		case "/cluster/status":
			if clusterStatusCalls.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"data":[
					{"type":"cluster","version":1,"quorate":1,"name":"cluster-one","id":"cluster-one","nodes":1},
					{"type":"node","name":"pve","id":"node/pve","nodeid":1,"online":1,"ip":"10.0.0.1"}
				]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[
				{"type":"cluster","version":2,"quorate":1,"name":"cluster-one","id":"cluster-one","nodes":1},
				{"type":"node","name":"edge","id":"node/edge","nodeid":2,"online":1,"ip":"10.0.0.2"}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	parent := task.GetTestTask(t)
	nodes := NewNodePool()
	SetCtx(parent, nodes)
	client := NewClient(srv.URL, goproxmox.WithHTTPClient(srv.Client()))
	require.NoError(t, client.UpdateClusterInfo(parent.Context()))

	fromContext, err := NodeFromCtx(parent.Context(), "pve")
	require.NoError(t, err)
	require.Same(t, client, fromContext.Client())
	require.Same(t, fromContext, client.nodes["pve"])

	otherNode := NewNode(NewClient("https://other.example/api2/json"), "pve", "node/pve")
	nodes.Add(otherNode)
	require.NoError(t, client.UpdateClusterInfo(parent.Context()))

	refreshedPVE, err := NodeFromCtx(parent.Context(), "pve")
	require.NoError(t, err)
	require.Same(t, otherNode, refreshedPVE)
	refreshedEdge, err := NodeFromCtx(parent.Context(), "edge")
	require.NoError(t, err)
	require.Same(t, client, refreshedEdge.Client())
	require.NotContains(t, client.nodes, "pve")
	require.Same(t, refreshedEdge, client.nodes["edge"])
}

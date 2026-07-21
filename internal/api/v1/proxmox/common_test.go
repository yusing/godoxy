package proxmoxapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/goutils/task"
)

func newNodeLookupTestContext(t *testing.T, nodes *proxmox.NodePool) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	taskCtx := task.GetTestTask(t)
	if nodes != nil {
		proxmox.SetCtx(taskCtx, nodes)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(taskCtx.Context())
	return c, recorder
}

func TestNodeFromRequestReturnsUniqueNode(t *testing.T) {
	nodes := proxmox.NewNodePool()
	client := proxmox.NewClient("https://pve.example/api2/json")
	node := proxmox.NewNode(client, "pve", "node/pve")
	nodes.Add(node)
	c, recorder := newNodeLookupTestContext(t, nodes)

	got, ok := nodeFromRequest(c, "pve")
	require.True(t, ok)
	require.Same(t, node, got)
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestNodeFromRequestReportsMissingAndAmbiguousNames(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		c, recorder := newNodeLookupTestContext(t, proxmox.NewNodePool())
		_, ok := nodeFromRequest(c, "missing")
		require.False(t, ok)
		require.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("ambiguous", func(t *testing.T) {
		nodes := proxmox.NewNodePool()
		nodes.Add(proxmox.NewNode(proxmox.NewClient("https://one.example/api2/json"), "pve", "node/pve"))
		nodes.Add(proxmox.NewNode(proxmox.NewClient("https://two.example/api2/json"), "pve", "node/pve"))
		c, recorder := newNodeLookupTestContext(t, nodes)
		_, ok := nodeFromRequest(c, "pve")
		require.False(t, ok)
		require.Equal(t, http.StatusConflict, recorder.Code)
	})
}

func TestNodeFromRequestReportsMissingContextAsInternalError(t *testing.T) {
	c, _ := newNodeLookupTestContext(t, nil)
	_, ok := nodeFromRequest(c, "pve")
	require.False(t, ok)
	require.Len(t, c.Errors, 1)
}

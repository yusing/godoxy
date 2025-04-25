package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/luthermonson/go-proxmox"
	"github.com/yusing/go-proxy/internal/utils/pool"
)

type Node struct {
	name   string
	id     string // likely node/<name>
	client *proxmox.Client
}

var Nodes = pool.New[*Node]("proxmox_nodes")

func AvailableNodeNames() string {
	var sb strings.Builder
	for _, node := range Nodes.Iter {
		sb.WriteString(node.name)
		sb.WriteString(", ")
	}
	return sb.String()[:sb.Len()-2]
}

func (n *Node) Key() string {
	return n.name
}

func (n *Node) Name() string {
	return n.name
}

func (n *Node) String() string {
	return fmt.Sprintf("%s (%s)", n.name, n.id)
}

func (n *Node) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"name": n.name,
		"id":   n.id,
	})
}

func (n *Node) Get(ctx context.Context, path string, v any) error {
	return n.client.Get(ctx, path, v)
}

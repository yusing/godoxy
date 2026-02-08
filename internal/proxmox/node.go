package proxmox

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/pool"
)

type NodeConfig struct {
	Node     string   `json:"node"`
	VMID     *int     `json:"vmid"` // unset: auto discover; explicit 0: node-level route; >0: lxc/qemu resource route
	VMName   string   `json:"vmname,omitempty"`
	Services []string `json:"services,omitempty" aliases:"service"`
	Files    []string `json:"files,omitempty" aliases:"file"`
} // @name ProxmoxNodeConfig

type Node struct {
	name   string
	id     string // likely node/<name>
	client *Client

	// statsScriptInitErrs *xsync.Map[int, error]
}

// Validate implements the serialization.CustomValidator interface.
func (n *NodeConfig) Validate() error {
	var errs gperr.Builder
	for i, service := range n.Services {
		if err := checkValidInput(service); err != nil {
			errs.AddSubjectf(err, "services[%d]", i)
		}
	}
	for i, file := range n.Files {
		if err := checkValidInput(file); err != nil {
			errs.AddSubjectf(err, "files[%d]", i)
		}
	}
	return errs.Error()
}

var Nodes = pool.New[*Node]("proxmox_nodes")

func NewNode(client *Client, name, id string) *Node {
	return &Node{
		name:   name,
		id:     id,
		client: client,
		// statsScriptInitErrs: xsync.NewMap[int, error](xsync.WithGrowOnly()),
	}
}

func AvailableNodeNames() string {
	if Nodes.Size() == 0 {
		return ""
	}
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

func (n *Node) Client() *Client {
	return n.client
}

func (n *Node) String() string {
	return fmt.Sprintf("%s (%s)", n.name, n.id)
}

func (n *Node) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"name": n.name,
		"id":   n.id,
	})
}

func (n *Node) Get(ctx context.Context, path string, v any) error {
	return n.client.Get(ctx, path, v)
}

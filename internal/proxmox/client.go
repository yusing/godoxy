package proxmox

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/luthermonson/go-proxmox"
	"github.com/rs/zerolog/log"
)

type Client struct {
	*proxmox.Client
	*proxmox.Cluster
	Version *proxmox.Version
	// id -> resource; id: lxc/<vmid> or qemu/<vmid>
	resources   map[string]*proxmox.ClusterResource
	resourcesMu sync.RWMutex
}

var (
	ErrResourceNotFound = errors.New("resource not found")
	ErrNoResources      = errors.New("no resources")
)

func NewClient(baseUrl string, opts ...proxmox.Option) *Client {
	return &Client{
		Client:    proxmox.NewClient(baseUrl, opts...),
		resources: make(map[string]*proxmox.ClusterResource),
	}
}

func (c *Client) UpdateClusterInfo(ctx context.Context) (err error) {
	c.Version, err = c.Client.Version(ctx)
	if err != nil {
		return err
	}
	// requires (/, Sys.Audit)
	cluster, err := c.Client.Cluster(ctx)
	if err != nil {
		return err
	}
	c.Cluster = cluster

	for _, node := range c.Cluster.Nodes {
		Nodes.Add(NewNode(c, node.Name, node.ID))
	}
	if cluster.Name == "" && len(c.Cluster.Nodes) == 1 {
		cluster.Name = c.Cluster.Nodes[0].Name
	}
	return nil
}

func (c *Client) UpdateResources(ctx context.Context) error {
	c.resourcesMu.Lock()
	defer c.resourcesMu.Unlock()
	resourcesSlice, err := c.Cluster.Resources(ctx, "vm")
	if err != nil {
		return err
	}
	clear(c.resources)
	for _, resource := range resourcesSlice {
		c.resources[resource.ID] = resource
	}
	log.Debug().Str("cluster", c.Cluster.Name).Msgf("[proxmox] updated %d resources", len(c.resources))
	return nil
}

// GetResource gets a resource by kind and id.
// kind: lxc or qemu
// id: <vmid>
func (c *Client) GetResource(kind string, id int) (*proxmox.ClusterResource, error) {
	c.resourcesMu.RLock()
	defer c.resourcesMu.RUnlock()
	resource, ok := c.resources[kind+"/"+strconv.Itoa(id)]
	if !ok {
		return nil, ErrResourceNotFound
	}
	return resource, nil
}

// Key implements pool.Object
func (c *Client) Key() string {
	return c.Cluster.ID
}

// Name implements pool.Object
func (c *Client) Name() string {
	return c.Cluster.Name
}

func (c *Client) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"version": c.Version,
		"cluster": map[string]any{
			"name":    c.Cluster.Name,
			"id":      c.Cluster.ID,
			"version": c.Cluster.Version,
			"nodes":   c.Cluster.Nodes,
			"quorate": c.Cluster.Quorate,
		},
	})
}

func (c *Client) NumNodes() int {
	return len(c.Cluster.Nodes)
}

func (c *Client) String() string {
	return fmt.Sprintf("%s (%s)", c.Cluster.Name, c.Cluster.ID)
}

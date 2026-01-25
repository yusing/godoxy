package proxmox

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/luthermonson/go-proxmox"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

type Client struct {
	*proxmox.Client
	*proxmox.Cluster
	Version *proxmox.Version
	// id -> resource; id: lxc/<vmid> or qemu/<vmid>
	resources   map[string]*VMResource
	resourcesMu sync.RWMutex
}

type VMResource struct {
	*proxmox.ClusterResource
	IPs []net.IP
}

var (
	ErrResourceNotFound = errors.New("resource not found")
	ErrNoResources      = errors.New("no resources")
)

func NewClient(baseUrl string, opts ...proxmox.Option) *Client {
	return &Client{
		Client:    proxmox.NewClient(baseUrl, opts...),
		resources: make(map[string]*VMResource),
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
	if c.Cluster == nil {
		return errors.New("cluster not initialized, call UpdateClusterInfo first")
	}
	resourcesSlice, err := c.Cluster.Resources(ctx, "vm")
	if err != nil {
		return err
	}
	vmResources := make([]*VMResource, len(resourcesSlice))
	for i, resource := range resourcesSlice {
		vmResources[i] = &VMResource{
			ClusterResource: resource,
			IPs:             nil,
		}
	}
	var errs errgroup.Group
	errs.SetLimit(runtime.GOMAXPROCS(0) * 2)
	for i, resource := range resourcesSlice {
		vmResource := vmResources[i]
		errs.Go(func() error {
			node, ok := Nodes.Get(resource.Node)
			if !ok {
				return fmt.Errorf("node %s not found", resource.Node)
			}
			vmid, ok := strings.CutPrefix(resource.ID, "lxc/")
			if !ok {
				return nil // not a lxc resource
			}
			vmidInt, err := strconv.Atoi(vmid)
			if err != nil {
				return fmt.Errorf("invalid resource id %s: %w", resource.ID, err)
			}
			ips, err := node.LXCGetIPs(ctx, vmidInt)
			if err != nil {
				return fmt.Errorf("failed to get ips for resource %s: %w", resource.ID, err)
			}
			vmResource.IPs = ips
			return nil
		})
	}
	if err := errs.Wait(); err != nil {
		return err
	}
	c.resourcesMu.Lock()
	clear(c.resources)
	for i, resource := range resourcesSlice {
		c.resources[resource.ID] = vmResources[i]
	}
	c.resourcesMu.Unlock()
	log.Debug().Str("cluster", c.Cluster.Name).Msgf("[proxmox] updated %d resources", len(c.resources))
	return nil
}

// GetResource gets a resource by kind and id.
// kind: lxc or qemu
// id: <vmid>
func (c *Client) GetResource(kind string, id int) (*VMResource, error) {
	c.resourcesMu.RLock()
	defer c.resourcesMu.RUnlock()
	resource, ok := c.resources[kind+"/"+strconv.Itoa(id)]
	if !ok {
		return nil, ErrResourceNotFound
	}
	return resource, nil
}

// ReverseLookupResource looks up a resource by ip address, hostname, alias or all of them
func (c *Client) ReverseLookupResource(ip net.IP, hostname string, alias string) (*VMResource, error) {
	c.resourcesMu.RLock()
	defer c.resourcesMu.RUnlock()

	shouldCheckIP := ip != nil && !ip.IsLoopback() && !ip.IsUnspecified()
	shouldCheckHostname := hostname != ""
	shouldCheckAlias := alias != ""

	if shouldCheckHostname {
		hostname, _, _ = strings.Cut(hostname, ".")
	}

	for _, resource := range c.resources {
		if shouldCheckIP && slices.ContainsFunc(resource.IPs, func(a net.IP) bool { return a.Equal(ip) }) {
			return resource, nil
		}
		if shouldCheckHostname && resource.Name == hostname {
			return resource, nil
		}
		if shouldCheckAlias && resource.Name == alias {
			return resource, nil
		}
	}
	return nil, ErrResourceNotFound
}

// ReverseLookupNode looks up a node by name or IP address.
// Returns the node name if found.
func (c *Client) ReverseLookupNode(hostname string, ip net.IP, alias string) string {
	shouldCheckHostname := hostname != ""
	shouldCheckIP := ip != nil && !ip.IsLoopback() && !ip.IsUnspecified()
	shouldCheckAlias := alias != ""

	if shouldCheckHostname {
		hostname, _, _ = strings.Cut(hostname, ".")
	}

	for _, node := range c.Cluster.Nodes {
		if shouldCheckHostname && node.Name == hostname {
			return node.Name
		}
		if shouldCheckIP {
			nodeIP := net.ParseIP(node.IP)
			if nodeIP != nil && nodeIP.Equal(ip) {
				return node.Name
			}
		}
		if shouldCheckAlias && node.Name == alias {
			return node.Name
		}
	}
	return ""
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

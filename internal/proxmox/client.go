package proxmox

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/luthermonson/go-proxmox"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

type Client struct {
	*proxmox.Client
	*proxmox.Cluster

	Version *proxmox.Version
	BaseURL *url.URL
	nodesMu sync.RWMutex
	nodes   map[string]*Node
	// id -> resource; id: lxc/<vmid> or qemu/<vmid>
	resources   map[string]*VMResource
	resourcesMu sync.RWMutex
}

type VMResource struct {
	*proxmox.ClusterResource

	IPs          []net.IP
	IPsFetchedAt time.Time
}

var (
	ErrResourceNotFound = errors.New("resource not found")
)

const lxcIPRefreshInterval = 30 * time.Second

func NewClient(baseURL string, opts ...proxmox.Option) *Client {
	return &Client{
		Client:    proxmox.NewClient(baseURL, opts...),
		nodes:     make(map[string]*Node),
		resources: make(map[string]*VMResource),
	}
}

func (c *Client) UpdateClusterInfo(ctx context.Context) (err error) {
	baseURL, err := url.Parse(c.Client.GetBaseURL())
	if err != nil {
		return err
	}
	c.BaseURL = baseURL
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

	nodePool := FromCtx(ctx)
	if nodePool == nil {
		return ErrNodePoolUnavailable
	}
	nodes := make(map[string]*Node, len(c.Cluster.Nodes))
	for _, nodeInfo := range c.Cluster.Nodes {
		node := NewNode(c, nodeInfo.Name, nodeInfo.ID)
		nodes[node.name] = node
	}
	c.nodesMu.Lock()
	c.nodes = nodes
	c.nodesMu.Unlock()
	nodePool.replaceProvider(c.Client.GetBaseURL(), nodes)
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
	c.resourcesMu.RLock()
	oldResources := maps.Clone(c.resources)
	c.resourcesMu.RUnlock()
	c.nodesMu.RLock()
	nodes := maps.Clone(c.nodes)
	c.nodesMu.RUnlock()

	now := time.Now()
	vmResources := make([]*VMResource, len(resourcesSlice))
	for i, resource := range resourcesSlice {
		oldResource := oldResources[resource.ID]
		var ips []net.IP
		var fetchedAt time.Time
		if oldResource != nil && oldResource.Status == resource.Status {
			ips = slices.Clone(oldResource.IPs)
			fetchedAt = oldResource.IPsFetchedAt
		}
		vmResources[i] = &VMResource{
			ClusterResource: resource,
			IPs:             ips,
			IPsFetchedAt:    fetchedAt,
		}
	}
	var workers errgroup.Group
	lookupErrs := gperr.NewGroup("failed to refresh proxmox resource IPs")
	limit := min(runtime.GOMAXPROCS(0), maxConcurrentResourceLookups)
	workers.SetLimit(limit)
	for i, resource := range resourcesSlice {
		vmResource := vmResources[i]
		oldResource := oldResources[resource.ID]
		workers.Go(func() error {
			vmid, ok := strings.CutPrefix(resource.ID, "lxc/")
			if !ok {
				return nil // not a lxc resource
			}
			node, ok := nodes[resource.Node]
			if !ok {
				lookupErrs.Addf("%s: node %s not found", resource.ID, resource.Node)
				return nil
			}
			vmidInt, err := strconv.Atoi(vmid)
			if err != nil {
				lookupErrs.Add(fmt.Errorf("invalid resource id %s: %w", resource.ID, err))
				return nil
			}

			if oldResource != nil &&
				oldResource.Status == resource.Status &&
				now.Sub(oldResource.IPsFetchedAt) < lxcIPRefreshInterval {
				return nil
			}

			ips, err := node.LXCGetIPsWithStatus(ctx, vmidInt, resource.Status)
			if err != nil {
				lookupErrs.Add(fmt.Errorf("%s: %w", resource.ID, err))
				return nil
			}
			vmResource.IPs = ips
			vmResource.IPsFetchedAt = now
			return nil
		})
	}
	_ = workers.Wait()
	c.resourcesMu.Lock()
	clear(c.resources)
	for i, resource := range resourcesSlice {
		c.resources[resource.ID] = vmResources[i]
	}
	c.resourcesMu.Unlock()
	return lookupErrs.Wait().Error()
}

// GetResource gets a resource by kind and id.
// kind: lxc or qemu
// id: <vmid>
func (c *Client) GetResource(kind string, id uint64) (*VMResource, error) {
	c.resourcesMu.RLock()
	defer c.resourcesMu.RUnlock()
	resource, ok := c.resources[kind+"/"+strconv.FormatUint(id, 10)]
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
	return strutils.MarshalJSON(map[string]any{
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

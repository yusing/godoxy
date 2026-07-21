package proxmox

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

type nodePoolContextKey struct{}

var (
	ErrNodePoolUnavailable = errors.New("proxmox node pool is unavailable")
	ErrNodeNotFound        = errors.New("proxmox node not found")
	ErrNodeAmbiguous       = errors.New("proxmox node name is ambiguous")
)

// NodePool contains the Proxmox nodes owned by one configuration state.
// Nodes from the same provider replace one another on refresh; nodes with the
// same name from different providers remain present so lookups can reject the
// ambiguity instead of choosing a provider nondeterministically.
type NodePool struct {
	mu     sync.RWMutex
	byName map[string]map[string]*Node
}

func NewNodePool() *NodePool {
	return &NodePool{byName: make(map[string]map[string]*Node)}
}

func SetCtx(ctx interface{ SetValue(key, value any) }, nodes *NodePool) {
	ctx.SetValue(nodePoolContextKey{}, nodes)
}

func FromCtx(ctx context.Context) *NodePool {
	if nodes, ok := ctx.Value(nodePoolContextKey{}).(*NodePool); ok {
		return nodes
	}
	return nil
}

func NodeFromCtx(ctx context.Context, name string) (*Node, error) {
	nodes := FromCtx(ctx)
	if nodes == nil {
		return nil, ErrNodePoolUnavailable
	}
	return nodes.Get(name)
}

func (p *NodePool) Add(node *Node) {
	p.mu.Lock()
	defer p.mu.Unlock()

	providers := p.byName[node.name]
	if providers == nil {
		providers = make(map[string]*Node)
		p.byName[node.name] = providers
	}
	providers[node.providerKey()] = node
}

// replaceProvider atomically removes a provider's previous nodes and inserts
// the nodes returned by its latest cluster refresh.
func (p *NodePool) replaceProvider(provider string, nodes map[string]*Node) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for name, providers := range p.byName {
		delete(providers, provider)
		if len(providers) == 0 {
			delete(p.byName, name)
		}
	}
	for name, node := range nodes {
		providers := p.byName[name]
		if providers == nil {
			providers = make(map[string]*Node)
			p.byName[name] = providers
		}
		providers[provider] = node
	}
}

func (p *NodePool) Get(name string) (*Node, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	providers := p.byName[name]
	switch len(providers) {
	case 0:
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, name)
	case 1:
		for _, node := range providers {
			return node, nil
		}
	}

	providerNames := make([]string, 0, len(providers))
	for provider := range providers {
		providerNames = append(providerNames, provider)
	}
	slices.Sort(providerNames)
	return nil, fmt.Errorf("%w: %s exists in providers %s", ErrNodeAmbiguous, name, strings.Join(providerNames, ", "))
}

func (p *NodePool) AvailableNodeNames() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	names := make([]string, 0, len(p.byName))
	for name, providers := range p.byName {
		if len(providers) == 1 {
			names = append(names, name)
			continue
		}
		for provider := range providers {
			names = append(names, name+"@"+provider)
		}
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

func AvailableNodeNames(ctx context.Context) string {
	nodes := FromCtx(ctx)
	if nodes == nil {
		return ""
	}
	return nodes.AvailableNodeNames()
}

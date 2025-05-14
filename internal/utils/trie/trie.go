package trie

import "github.com/puzpuzpuz/xsync/v4"

type Root struct {
	*Node
	cached *xsync.Map[string, *Node]
}

func NewTrie() *Root {
	return &Root{
		Node: &Node{
			children: xsync.NewMap[string, *Node](),
		},
		cached: xsync.NewMap[string, *Node](),
	}
}

func (r *Root) getNode(key *Key, newFunc func() any) *Node {
	if key.hasWildcard {
		panic("should not call Load or Store on a key with any wildcard: " + key.full)
	}
	node, _ := r.cached.LoadOrCompute(key.full, func() (*Node, bool) {
		return r.Node.loadOrStore(key, newFunc)
	})
	return node
}

// LoadOrStore loads or stores the value for the key
// Returns the value loaded/stored
func (r *Root) LoadOrStore(key *Key, newFunc func() any) any {
	return r.getNode(key, newFunc).value.Load()
}

// LoadAndStore loads or stores the value for the key
// Returns the old value if exists, nil otherwise
func (r *Root) LoadAndStore(key *Key, val any) any {
	return r.getNode(key, func() any { return val }).value.Swap(val)
}

// Store stores the value for the key
func (r *Root) Store(key *Key, val any) {
	r.getNode(key, func() any { return val }).value.Store(val)
}

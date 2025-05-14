package trie

import (
	"github.com/puzpuzpuz/xsync/v4"
)

type Node struct {
	key      string
	children *xsync.Map[string, *Node] // lock-free map which allows concurrent access
	value    AnyValue                  // only end nodes have values
}

func mayPrefix(key, part string) string {
	if key == "" {
		return part
	}
	return key + "." + part
}

func (node *Node) newChild(part string) *Node {
	return &Node{
		key:      mayPrefix(node.key, part),
		children: xsync.NewMap[string, *Node](),
	}
}

func (node *Node) Get(key *Key) (any, bool) {
	for _, seg := range key.segments {
		child, ok := node.children.Load(seg)
		if !ok {
			return nil, false
		}
		node = child
	}
	v := node.value.Load()
	if v == nil {
		return nil, false
	}
	return v, true
}

func (node *Node) loadOrStore(key *Key, newFunc func() any) (*Node, bool) {
	for i, seg := range key.segments {
		child, _ := node.children.LoadOrCompute(seg, func() (*Node, bool) {
			newNode := node.newChild(seg)
			if i == len(key.segments)-1 {
				newNode.value.Store(newFunc())
			}
			return newNode, false
		})
		node = child
	}
	return node, false
}

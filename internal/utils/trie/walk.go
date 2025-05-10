package trie

import (
	"maps"
	"slices"
)

type (
	YieldFunc    = func(part string, value any) bool
	YieldKeyFunc = func(key string) bool
	Iterator     = func(YieldFunc)
	KeyIterator  = func(YieldKeyFunc)
)

// WalkAll walks all nodes in the trie, yields full key and series
func (node *Node) Walk(yield YieldFunc) {
	node.walkAll(yield)
}

func (node *Node) walkAll(yield YieldFunc) bool {
	if !node.value.IsNil() {
		return yield(node.key, node.value.Load())
	}
	for _, v := range node.children.Range {
		if !v.walkAll(yield) {
			return false
		}
	}
	return true
}

func (node *Node) WalkKeys(yield YieldKeyFunc) {
	node.walkKeys(yield)
}

func (node *Node) walkKeys(yield YieldKeyFunc) bool {
	if !node.value.IsNil() {
		return !yield(node.key)
	}
	for _, v := range node.children.Range {
		if !v.walkKeys(yield) {
			return false
		}
	}
	return true
}

func (node *Node) Keys() []string {
	return slices.Collect(node.WalkKeys)
}

func (node *Node) Map() map[string]any {
	return maps.Collect(node.Walk)
}

func (tree Root) Query(key *Key) Iterator {
	if !key.hasWildcard {
		return func(yield YieldFunc) {
			if v, ok := tree.Get(key); ok {
				yield(key.full, v)
			}
		}
	}
	return func(yield YieldFunc) {
		tree.walkQuery(key.segments, tree.Node, yield, false)
	}
}

func (tree Root) walkQuery(patternParts []string, node *Node, yield YieldFunc, recursive bool) bool {
	if len(patternParts) == 0 {
		if !node.value.IsNil() { // end
			if !yield(node.key, node.value.Load()) {
				return true
			}
		} else if recursive {
			return tree.walkAll(yield)
		}
		return true
	}
	pat := patternParts[0]

	switch pat {
	case "**":
		// ** matches zero or more segments
		// Option 1: ** matches zero segment, move to next pattern part
		if !tree.walkQuery(patternParts[1:], node, yield, false) {
			return false
		}
		// Option 2: ** matches one or more segments
		for _, child := range node.children.Range {
			if !tree.walkQuery(patternParts, child, yield, true) {
				return false
			}
		}
	case "*":
		// * matches any single segment
		for _, child := range node.children.Range {
			if !tree.walkQuery(patternParts[1:], child, yield, false) {
				return false
			}
		}
	default:
		// Exact match
		if child, ok := node.children.Load(pat); ok {
			return tree.walkQuery(patternParts[1:], child, yield, false)
		}
	}
	return true
}

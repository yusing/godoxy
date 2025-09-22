package trie

import (
	"slices"
	"strings"

	"github.com/yusing/godoxy/internal/utils/strutils"
)

type Key struct {
	segments    []string // escaped segments
	full        string   // unescaped original key
	hasWildcard bool
}

func Namespace(ns string) *Key {
	return &Key{
		segments:    []string{ns},
		full:        ns,
		hasWildcard: false,
	}
}

func NewKey(keyStr string) *Key {
	key := &Key{
		segments: strutils.SplitRune(keyStr, '.'),
		full:     keyStr,
	}
	for _, seg := range key.segments {
		if seg == "*" || seg == "**" {
			key.hasWildcard = true
		}
	}
	return key
}

func EscapeSegment(seg string) string {
	var sb strings.Builder
	for _, r := range seg {
		switch r {
		case '.', '*':
			sb.WriteString("__")
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func (ns Key) With(segment string) *Key {
	ns.segments = append(ns.segments, segment)
	ns.full = ns.full + "." + segment
	ns.hasWildcard = ns.hasWildcard || segment == "*" || segment == "**"
	return &ns
}

func (ns Key) WithEscaped(segment string) *Key {
	ns.segments = append(ns.segments, EscapeSegment(segment))
	ns.full = ns.full + "." + segment
	return &ns
}

func (ns *Key) NumSegments() int {
	return len(ns.segments)
}

func (ns *Key) HasWildcard() bool {
	return ns.hasWildcard
}

func (ns *Key) String() string {
	return ns.full
}

func (ns *Key) Clone() *Key {
	clone := *ns
	clone.segments = slices.Clone(ns.segments)
	clone.full = strings.Clone(ns.full)
	return &clone
}

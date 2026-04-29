package autocert

import (
	"crypto/tls"
	"crypto/x509"
	"strings"
)

type certSource interface {
	getTLSCert() *tls.Certificate
}

type sniMatcher struct {
	exact map[string]certSource
	root  sniTreeNode
}

type sniTreeNode struct {
	children map[string]*sniTreeNode
	wildcard certSource
}

func (m *sniMatcher) match(serverName string) certSource {
	if m == nil {
		return nil
	}
	serverName = normalizeServerName(serverName)
	if serverName == "" {
		return nil
	}
	if m.exact != nil {
		if p, ok := m.exact[serverName]; ok {
			return p
		}
	}
	return m.matchSuffixTree(serverName)
}

func (m *sniMatcher) matchSuffixTree(serverName string) certSource {
	n := &m.root
	labels := strings.Split(serverName, ".")

	var best certSource
	for i := len(labels) - 1; i >= 0; i-- {
		if n.children == nil {
			break
		}
		next := n.children[labels[i]]
		if next == nil {
			break
		}
		n = next

		consumed := len(labels) - i
		remaining := len(labels) - consumed
		if remaining == 1 && n.wildcard != nil {
			best = n.wildcard
		}
	}
	return best
}

func normalizeServerName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".")
	return strings.ToLower(s)
}

func (m *sniMatcher) addProvider(p certSource) {
	if p == nil {
		return
	}
	tlsCert := p.getTLSCert()
	if tlsCert == nil || len(tlsCert.Certificate) == 0 {
		return
	}
	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return
	}

	addName := func(name string) {
		name = normalizeServerName(name)
		if name == "" {
			return
		}
		if after, ok := strings.CutPrefix(name, "*."); ok {
			suffix := after
			if suffix == "" {
				return
			}
			m.insertWildcardSuffix(suffix, p)
			return
		}
		m.insertExact(name, p)
	}

	if leaf.Subject.CommonName != "" {
		addName(leaf.Subject.CommonName)
	}
	for _, n := range leaf.DNSNames {
		addName(n)
	}
}

func (m *sniMatcher) insertExact(name string, p certSource) {
	if name == "" || p == nil {
		return
	}
	if m.exact == nil {
		m.exact = make(map[string]certSource)
	}
	if _, exists := m.exact[name]; !exists {
		m.exact[name] = p
	}
}

func (m *sniMatcher) insertWildcardSuffix(suffix string, p certSource) {
	if suffix == "" || p == nil {
		return
	}
	n := &m.root
	labels := strings.Split(suffix, ".")
	for i := len(labels) - 1; i >= 0; i-- {
		if n.children == nil {
			n.children = make(map[string]*sniTreeNode)
		}
		next := n.children[labels[i]]
		if next == nil {
			next = &sniTreeNode{}
			n.children[labels[i]] = next
		}
		n = next
	}
	if n.wildcard == nil {
		n.wildcard = p
	}
}

package entrypoint

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type preferredFakeHTTPRoute struct {
	*fakeHTTPRoute
	prefer bool
}

func (r *preferredFakeHTTPRoute) PreferOver(any) bool {
	return r.prefer
}

func TestWildcardRouteIndexAddResolvesConflictingSuffix(t *testing.T) {
	idx := newWildcardRouteIndex()
	existing := newFakeHTTPRoute(t, "*.example.com", "")
	replacement := &preferredFakeHTTPRoute{
		fakeHTTPRoute: newFakeHTTPRoute(t, "*.example.com.", ""),
		prefer:        true,
	}

	idx.Add(existing)
	idx.Add(replacement)

	require.Same(t, replacement, idx.Find("app.example.com"))
}

func TestWildcardRouteIndexAddKeepsPreferredExistingRoute(t *testing.T) {
	idx := newWildcardRouteIndex()
	existing := &preferredFakeHTTPRoute{
		fakeHTTPRoute: newFakeHTTPRoute(t, "*.example.com", ""),
		prefer:        true,
	}
	shadowed := newFakeHTTPRoute(t, "*.example.com.", "")

	idx.Add(existing)
	idx.Add(shadowed)

	require.Same(t, existing, idx.Find("app.example.com"))
}

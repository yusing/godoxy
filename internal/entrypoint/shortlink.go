package entrypoint

import (
	"net/http"
	"strings"

	"github.com/puzpuzpuz/xsync/v4"
)

type ShortLinkMatcher struct {
	defaultDomainSuffix string // e.g. ".example.com"

	fqdnRoutes      *xsync.Map[string, string] // "app" -> "app.example.com"
	subdomainRoutes *xsync.Map[string, struct{}]
}

func newShortLinkMatcher() *ShortLinkMatcher {
	return &ShortLinkMatcher{
		fqdnRoutes:      xsync.NewMap[string, string](),
		subdomainRoutes: xsync.NewMap[string, struct{}](),
	}
}

func (st *ShortLinkMatcher) SetDefaultDomainSuffix(suffix string) {
	if !strings.HasPrefix(suffix, ".") {
		suffix = "." + suffix
	}
	st.defaultDomainSuffix = suffix
}

func (st *ShortLinkMatcher) AddRoute(alias string) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return
	}

	if strings.Contains(alias, ".") { // FQDN alias
		st.fqdnRoutes.Store(alias, alias)
		key, _, _ := strings.Cut(alias, ".")
		if key != "" {
			if _, ok := st.subdomainRoutes.Load(key); !ok {
				if _, ok := st.fqdnRoutes.Load(key); !ok {
					st.fqdnRoutes.Store(key, alias)
				}
			}
		}
		return
	}

	// subdomain alias + defaultDomainSuffix
	if st.defaultDomainSuffix == "" {
		return
	}
	st.subdomainRoutes.Store(alias, struct{}{})
}

func (st *ShortLinkMatcher) DelRoute(alias string) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return
	}

	if strings.Contains(alias, ".") {
		st.fqdnRoutes.Delete(alias)
		key, _, _ := strings.Cut(alias, ".")
		if key != "" {
			if target, ok := st.fqdnRoutes.Load(key); ok && target == alias {
				st.fqdnRoutes.Delete(key)
			}
		}
		return
	}

	st.subdomainRoutes.Delete(alias)
}

func (st *ShortLinkMatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.EscapedPath()
	trim := strings.TrimPrefix(path, "/")
	key, rest, _ := strings.Cut(trim, "/")
	if key == "" {
		http.Error(w, "short link key is required", http.StatusBadRequest)
		return
	}
	if rest != "" {
		rest = "/" + rest
	} else {
		rest = "/"
	}

	targetHost := ""
	if strings.Contains(key, ".") {
		targetHost, _ = st.fqdnRoutes.Load(key)
	} else if target, ok := st.fqdnRoutes.Load(key); ok {
		targetHost = target
	} else if _, ok := st.subdomainRoutes.Load(key); ok && st.defaultDomainSuffix != "" {
		targetHost = key + st.defaultDomainSuffix
	}

	if targetHost == "" {
		http.Error(w, "short link not found", http.StatusNotFound)
		return
	}

	targetURL := "https://" + targetHost + rest
	if q := r.URL.RawQuery; q != "" {
		targetURL += "?" + q
	}
	http.Redirect(w, r, targetURL, http.StatusTemporaryRedirect)
}

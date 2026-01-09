package icons

import "sync/atomic"

type Provider interface {
	HasIcon(u *URL) bool
}

var provider atomic.Value

func SetProvider(p Provider) {
	provider.Store(p)
}

func hasIcon(u *URL) bool {
	v := provider.Load()
	if v == nil {
		return false
	}
	return v.(Provider).HasIcon(u)
}

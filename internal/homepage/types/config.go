package homepage

import "sync/atomic"

type Config struct {
	UseDefaultCategories bool `json:"use_default_categories"`
}

// nil-safe
var ActiveConfig atomic.Pointer[Config]

func init() {
	ActiveConfig.Store(&Config{
		UseDefaultCategories: true,
	})
}

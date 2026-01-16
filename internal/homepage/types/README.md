# Types Package

Configuration types for the homepage package.

## Config

```go
type Config struct {
    UseDefaultCategories bool `json:"use_default_categories"`
}

var ActiveConfig atomic.Pointer[Config]
```

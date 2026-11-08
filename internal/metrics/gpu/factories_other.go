//go:build !linux

package gpu

var factories = map[string]func() (Collector, error){}

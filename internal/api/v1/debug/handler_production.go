//go:build !debug

package debugapi

import (
	config "github.com/yusing/go-proxy/internal/config/types"
)

func StartServer(cfg config.ConfigInstance) {
	// do nothing
}

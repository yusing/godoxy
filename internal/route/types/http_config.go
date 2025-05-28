package route

import (
	"time"
)

type HTTPConfig struct {
	NoTLSVerify           bool          `json:"no_tls_verify,omitempty"`
	ResponseHeaderTimeout time.Duration `json:"response_header_timeout,omitempty"`
	DisableCompression    bool          `json:"disable_compression,omitempty"`
}

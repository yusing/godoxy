package proxmox

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"
	"github.com/yusing/godoxy/internal/net/gphttp"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

type Config struct {
	URL string `json:"url" validate:"required,url"`

	TokenID string            `json:"token_id" validate:"required"`
	Secret  strutils.Redacted `json:"secret" validate:"required"`

	NoTLSVerify bool `json:"no_tls_verify" yaml:"no_tls_verify,omitempty"`

	client *Client
}

func (c *Config) Client() *Client {
	if c.client == nil {
		panic("proxmox client accessed before init")
	}
	return c.client
}

func (c *Config) Init() gperr.Error {
	var tr *http.Transport
	if c.NoTLSVerify {
		// user specified
		tr = gphttp.NewTransportWithTLSConfig(&tls.Config{
			InsecureSkipVerify: true, //nolint:gosec
		})
	} else {
		tr = gphttp.NewTransport()
	}

	c.URL = strings.TrimSuffix(c.URL, "/")
	if !strings.HasSuffix(c.URL, "/api2/json") {
		c.URL += "/api2/json"
	}

	opts := []proxmox.Option{
		proxmox.WithAPIToken(c.TokenID, c.Secret.String()),
		proxmox.WithHTTPClient(&http.Client{
			Transport: tr,
		}),
	}
	c.client = NewClient(c.URL, opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := c.client.UpdateClusterInfo(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return gperr.New("timeout fetching proxmox cluster info")
		}
		return gperr.New("failed to fetch proxmox cluster info").With(err)
	}
	return nil
}

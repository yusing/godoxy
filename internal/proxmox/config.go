package proxmox

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/luthermonson/go-proxmox"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/net/gphttp"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

type Config struct {
	URL string `json:"url" validate:"required,url"`

	Username string            `json:"username" validate:"required_without_all=TokenID Secret"`
	Password strutils.Redacted `json:"password" validate:"required_without_all=TokenID Secret"`
	Realm    string            `json:"realm"` // default is "pam"

	TokenID string            `json:"token_id" validate:"required_without_all=Username Password"`
	Secret  strutils.Redacted `json:"secret" validate:"required_without_all=Username Password"`

	NoTLSVerify bool `json:"no_tls_verify" yaml:"no_tls_verify,omitempty"`

	client *Client
}

const ResourcePollInterval = 3 * time.Second

// NodeStatsPollInterval controls how often node stats are streamed when streaming is enabled.
const NodeStatsPollInterval = time.Second

func (c *Config) Client() *Client {
	if c.client == nil {
		panic("proxmox client accessed before init")
	}
	return c.client
}

func (c *Config) Init(ctx context.Context) gperr.Error {
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
		proxmox.WithHTTPClient(&http.Client{
			Transport: tr,
		}),
	}
	useCredentials := false
	if c.Username != "" && c.Password != "" {
		if c.Realm == "" {
			c.Realm = "pam"
		}
		opts = append(opts, proxmox.WithCredentials(&proxmox.Credentials{
			Username: c.Username,
			Password: c.Password.String(),
			Realm:    c.Realm,
		}))
		useCredentials = true
	} else {
		opts = append(opts, proxmox.WithAPIToken(c.TokenID, c.Secret.String()))
	}
	c.client = NewClient(c.URL, opts...)

	initCtx, initCtxCancel := context.WithTimeout(ctx, 5*time.Second)
	defer initCtxCancel()

	if useCredentials {
		err := c.client.CreateSession(initCtx)
		if err != nil {
			return gperr.New("failed to create session").With(err)
		}
	}

	if err := c.client.UpdateClusterInfo(initCtx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return gperr.New("timeout fetching proxmox cluster info")
		}
		return gperr.New("failed to fetch proxmox cluster info").With(err)
	}

	{
		reqCtx, reqCtxCancel := context.WithTimeout(ctx, ResourcePollInterval)
		err := c.client.UpdateResources(reqCtx)
		reqCtxCancel()
		if err != nil {
			log.Warn().Err(err).Str("cluster", c.client.Cluster.Name).Msg("[proxmox] failed to update resources")
		}
	}

	go c.updateResourcesLoop(ctx)
	return nil
}

func (c *Config) updateResourcesLoop(ctx context.Context) {
	ticker := time.NewTicker(ResourcePollInterval)
	defer ticker.Stop()

	log.Trace().Str("cluster", c.client.Cluster.Name).Msg("[proxmox] starting resources update loop")

	for {
		select {
		case <-ctx.Done():
			log.Trace().Str("cluster", c.client.Cluster.Name).Msg("[proxmox] stopping resources update loop")
			return
		case <-ticker.C:
			reqCtx, reqCtxCancel := context.WithTimeout(ctx, ResourcePollInterval)
			err := c.client.UpdateResources(reqCtx)
			reqCtxCancel()
			if err != nil {
				log.Error().Err(err).Str("cluster", c.client.Cluster.Name).Msg("[proxmox] failed to update resources")
			}
		}
	}
}

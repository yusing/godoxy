package docker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/client"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
)

type (
	SharedClient struct {
		*client.Client

		key      string
		refCount uint32
		closedOn int64

		addr string
		dial func(ctx context.Context) (net.Conn, error)
	}
)

var (
	clientMap   = make(map[string]*SharedClient, 10)
	clientMapMu sync.RWMutex

	clientOptEnvHost = []client.Opt{
		client.WithHostFromEnv(),
		client.WithAPIVersionNegotiation(),
	}
)

const (
	cleanInterval = 10 * time.Second
	clientTTLSecs = int64(10)
)

func init() {
	cleaner := task.RootTask("docker_clients_cleaner")
	go func() {
		ticker := time.NewTicker(cleanInterval)
		defer ticker.Stop()
		defer cleaner.Finish("program exit")

		for {
			select {
			case <-ticker.C:
				closeTimedOutClients()
			case <-cleaner.Context().Done():
				return
			}
		}
	}()

	task.OnProgramExit("docker_clients_cleanup", func() {
		clientMapMu.Lock()
		defer clientMapMu.Unlock()

		for _, c := range clientMap {
			delete(clientMap, c.key)
			c.Client.Close()
		}
	})
}

func closeTimedOutClients() {
	clientMapMu.Lock()
	defer clientMapMu.Unlock()

	now := time.Now().Unix()

	for _, c := range clientMap {
		if atomic.LoadUint32(&c.refCount) == 0 && now-atomic.LoadInt64(&c.closedOn) > clientTTLSecs {
			delete(clientMap, c.key)
			c.Client.Close()
			logging.Debug().Str("host", c.key).Msg("docker client closed")
		}
	}
}

func (c *SharedClient) Address() string {
	return c.addr
}

func (c *SharedClient) CheckConnection(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

// if the client is still referenced, this is no-op.
func (c *SharedClient) Close() {
	atomic.StoreInt64(&c.closedOn, time.Now().Unix())
	atomic.AddUint32(&c.refCount, ^uint32(0))
}

// NewClient creates a new Docker client connection to the specified host.
//
// Returns existing client if available.
//
// Parameters:
//   - host: the host to connect to (either a URL or common.DockerHostFromEnv).
//
// Returns:
//   - Client: the Docker client connection.
//   - error: an error if the connection failed.
func NewClient(host string) (*SharedClient, error) {
	clientMapMu.Lock()
	defer clientMapMu.Unlock()

	if client, ok := clientMap[host]; ok {
		atomic.StoreInt64(&client.closedOn, 0)
		atomic.AddUint32(&client.refCount, 1)
		return client, nil
	}

	// create client
	var opt []client.Opt
	var addr string
	var dial func(ctx context.Context) (net.Conn, error)

	if agent.IsDockerHostAgent(host) {
		cfg, ok := config.GetInstance().GetAgent(host)
		if !ok {
			panic(fmt.Errorf("agent %q not found", host))
		}
		opt = []client.Opt{
			client.WithHost(agent.DockerHost),
			client.WithHTTPClient(cfg.NewHTTPClient()),
			client.WithAPIVersionNegotiation(),
		}
		addr = "tcp://" + cfg.Addr
		dial = cfg.DialContext
	} else {
		switch host {
		case "":
			return nil, errors.New("empty docker host")
		case common.DockerHostFromEnv:
			opt = clientOptEnvHost
		default:
			helper, err := connhelper.GetConnectionHelper(host)
			if err != nil {
				logging.Panic().Err(err).Msg("failed to get connection helper")
			}
			if helper != nil {
				httpClient := &http.Client{
					Transport: &http.Transport{
						DialContext: helper.Dialer,
					},
				}
				opt = []client.Opt{
					client.WithHTTPClient(httpClient),
					client.WithHost(helper.Host),
					client.WithAPIVersionNegotiation(),
					client.WithDialContext(helper.Dialer),
				}
			} else {
				opt = []client.Opt{
					client.WithHost(host),
					client.WithAPIVersionNegotiation(),
				}
			}
		}
	}

	client, err := client.NewClientWithOpts(opt...)
	if err != nil {
		return nil, err
	}

	c := &SharedClient{
		Client:   client,
		key:      host,
		refCount: 1,
		addr:     addr,
		dial:     dial,
	}

	// non-agent client
	if c.dial == nil {
		c.dial = client.Dialer()
	}

	defer logging.Debug().Str("host", host).Msg("docker client initialized")

	clientMap[c.key] = c
	return c, nil
}

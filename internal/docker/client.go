package docker

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/task"
)

// TODO: implement reconnect here.
type (
	SharedClient struct {
		*client.Client

		refCount uint32
		closedOn int64

		key  string
		addr string
		dial func(ctx context.Context) (net.Conn, error)
	}
)

var (
	clientMap   = make(map[string]*SharedClient, 10)
	clientMapMu sync.RWMutex
)

var initClientCleanerOnce sync.Once

const (
	cleanInterval = 10 * time.Second
	clientTTLSecs = int64(10)
)

func initClientCleaner() {
	cleaner := task.RootTask("docker_clients_cleaner", true)
	go func() {
		ticker := time.NewTicker(cleanInterval)
		defer ticker.Stop()
		defer cleaner.Finish("program exit")

		for {
			select {
			case <-ticker.C:
				closeTimedOutClients()
			case <-cleaner.Context().Done():
				clientMapMu.Lock()
				for _, c := range clientMap {
					delete(clientMap, c.Key())
					c.Client.Close()
				}
				clientMapMu.Unlock()
				return
			}
		}
	}()
}

func closeTimedOutClients() {
	clientMapMu.Lock()
	defer clientMapMu.Unlock()

	now := time.Now().Unix()

	for _, c := range clientMap {
		if atomic.LoadUint32(&c.refCount) == 0 && now-atomic.LoadInt64(&c.closedOn) > clientTTLSecs {
			delete(clientMap, c.Key())
			c.Client.Close()
			log.Debug().Str("host", c.DaemonHost()).Msg("docker client closed")
		}
	}
}

func Clients() map[string]*SharedClient {
	clientMapMu.RLock()
	defer clientMapMu.RUnlock()

	clients := make(map[string]*SharedClient, len(clientMap))
	maps.Copy(clients, clientMap)
	return clients
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
	initClientCleanerOnce.Do(initClientCleaner)

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
		cfg, ok := agent.GetAgent(host)
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
			opt = []client.Opt{
				client.WithHostFromEnv(),
				client.WithAPIVersionNegotiation(),
			}
		default:
			helper, err := connhelper.GetConnectionHelper(host)
			if err != nil {
				log.Panic().Err(err).Msg("failed to get connection helper")
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
		refCount: 1,
		addr:     addr,
		key:      host,
		dial:     dial,
	}

	// non-agent client
	if c.dial == nil {
		c.dial = client.Dialer()
	}
	if c.addr == "" {
		c.addr = c.DaemonHost()
	}

	defer log.Debug().Str("host", host).Msg("docker client initialized")

	clientMap[c.Key()] = c
	return c, nil
}

func (c *SharedClient) Key() string {
	return c.key
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

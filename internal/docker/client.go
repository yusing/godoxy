package docker

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/docker/cli/cli/connhelper"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/types"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/task"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// TODO: implement reconnect here.
type (
	SharedClient struct {
		*client.Client

		cfg types.DockerProviderConfig

		refCount atomic.Int32
		closedOn atomic.Int64

		key  string
		addr string
		dial func(ctx context.Context) (net.Conn, error)

		unique bool
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
		if c.refCount.Load() == 0 && now-c.closedOn.Load() > clientTTLSecs {
			delete(clientMap, c.Key())
			c.Client.Close()
			log.Debug().Str("host", c.DaemonHost()).Msg("docker client closed")
		}
	}
}

// Clients return a map of currently connected clients.
// Close() must be called on all these clients after use.
func Clients() map[string]*SharedClient {
	clientMapMu.RLock()

	clients := make(map[string]*SharedClient, len(clientMap))
	maps.Copy(clients, clientMap)
	clientMapMu.RUnlock()

	// add 1 ref count to prevent them from
	// being closed before caller finished using them
	for _, c := range clients {
		// last Close() has been called, reset closeOn
		if c.refCount.Add(1) == 1 {
			c.closedOn.Store(0)
		}
	}
	return clients
}

// NewClient creates a new Docker client connection to the specified host.
//
// Returns existing client if available.
//
// Parameters:
//   - host: the host to connect to (either a URL or client.DefaultDockerHost).
//
// Returns:
//   - Client: the Docker client connection.
//   - error: an error if the connection failed.
func NewClient(cfg types.DockerProviderConfig, unique ...bool) (*SharedClient, error) {
	initClientCleanerOnce.Do(initClientCleaner)

	u := false
	if len(unique) > 0 {
		u = unique[0]
	}

	host := cfg.URL

	if !u {
		clientMapMu.Lock()
		defer clientMapMu.Unlock()

		if client, ok := clientMap[host]; ok {
			client.closedOn.Store(0)
			client.refCount.Add(1)
			return client, nil
		}
	}

	// create client
	var opt []client.Opt
	var addr string
	var dial func(ctx context.Context) (net.Conn, error)

	if agent.IsDockerHostAgent(host) {
		a, ok := agentpool.Get(host)
		if !ok {
			return nil, fmt.Errorf("agent %q not found", host)
		}
		opt = []client.Opt{
			client.WithHost(agent.DockerHost),
			client.WithHTTPClient(a.HTTPClient()),
		}
		addr = "tcp://" + a.Addr
		dial = a.DialContext
	} else {
		helper, err := connhelper.GetConnectionHelper(host)
		if err != nil {
			log.Panic().Err(err).Msg("failed to get connection helper")
		}
		if helper != nil {
			opt = []client.Opt{
				client.WithHost(helper.Host),
				client.WithDialContext(helper.Dialer),
			}
		} else {
			// connhelper.GetConnectionHelper already parsed the host without error
			url, _ := url.Parse(host)
			opt = []client.Opt{
				client.WithHost(host),
			}
			switch url.Scheme {
			case "", "tls", "http", "https":
				if (url.Scheme == "https" || url.Scheme == "tls") && cfg.TLS == nil {
					return nil, fmt.Errorf("TLS config is not set when using %s:// host", url.Scheme)
				}

				dial = func(ctx context.Context) (net.Conn, error) {
					var dialer net.Dialer
					return dialer.DialContext(ctx, "tcp", url.Host)
				}

				opt = append(opt, client.WithDialContext(func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dial(ctx)
				}))
			}
		}
	}

	if cfg.TLS != nil {
		opt = append(opt, client.WithTLSClientConfig(cfg.TLS.CAFile, cfg.TLS.CertFile, cfg.TLS.KeyFile))
	}

	opt = append(opt, client.WithAPIVersionNegotiation())

	client, err := client.NewClientWithOpts(opt...)
	if err != nil {
		return nil, err
	}

	c := &SharedClient{
		Client: client,
		cfg:    cfg,
		addr:   addr,
		key:    host,
		dial:   dial,
		unique: u,
	}
	c.unotel()
	c.refCount.Store(1)

	// non-agent client
	if c.dial == nil {
		c.dial = client.Dialer()
	}
	if c.addr == "" {
		c.addr = c.DaemonHost()
	}

	defer log.Debug().Str("host", host).Msg("docker client initialized")

	if !u {
		clientMap[c.Key()] = c
	}
	return c, nil
}

func (c *SharedClient) GetHTTPClient() **http.Client {
	return (**http.Client)(unsafe.Add(unsafe.Pointer(c.Client), clientClientOffset))
}

func (c *SharedClient) InterceptHTTPClient(intercept httputils.InterceptFunc) {
	httpClient := *c.GetHTTPClient()
	httpClient.Transport = httputils.NewInterceptedTransport(httpClient.Transport, intercept)
}

func (c *SharedClient) CloneUnique() *SharedClient {
	// there will be no error here
	// since we are using the same host from a valid client.
	c, _ = NewClient(c.cfg, true)
	return c
}

func (c *SharedClient) Key() string {
	return c.key
}

func (c *SharedClient) DaemonHost() string {
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

// for shared clients, if the client is still referenced, this is no-op.
func (c *SharedClient) Close() {
	if c.unique {
		c.Client.Close()
		return
	}
	c.closedOn.Store(time.Now().Unix())
	c.refCount.Add(-1)
}

var clientClientOffset = func() uintptr {
	field, ok := reflect.TypeFor[client.Client]().FieldByName("client")
	if !ok {
		panic("client.Client has no client field")
	}
	return field.Offset
}()

var otelRtOffset = func() uintptr {
	field, ok := reflect.TypeFor[otelhttp.Transport]().FieldByName("rt")
	if !ok {
		panic("otelhttp.Transport has no rt field")
	}
	return field.Offset
}()

func (c *SharedClient) unotel() {
	// we don't need and don't want otelhttp.Transport here.
	httpClient := *c.GetHTTPClient()

	otelTransport, ok := httpClient.Transport.(*otelhttp.Transport)
	if !ok {
		log.Debug().Str("host", c.DaemonHost()).Msgf("docker client transport is not an otelhttp.Transport: %T", httpClient.Transport)
		return
	}
	transport := *(*http.RoundTripper)(unsafe.Add(unsafe.Pointer(otelTransport), otelRtOffset))
	httpClient.Transport = transport
}

package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/yusing/go-proxy/agent/pkg/certs"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/pkg"
)

type AgentConfig struct {
	Addr string

	httpClient *http.Client
	tlsConfig  *tls.Config
	name       string
}

const (
	EndpointVersion    = "/version"
	EndpointName       = "/name"
	EndpointProxyHTTP  = "/proxy/http"
	EndpointHealth     = "/health"
	EndpointLogs       = "/logs"
	EndpointSystemInfo = "/system_info"

	AgentHost = CertsDNSName

	APIEndpointBase = "/godoxy/agent"
	APIBaseURL      = "https://" + AgentHost + APIEndpointBase

	DockerHost = "https://" + AgentHost

	FakeDockerHostPrefix    = "agent://"
	FakeDockerHostPrefixLen = len(FakeDockerHostPrefix)
)

var (
	AgentURL, _           = url.Parse(APIBaseURL)
	HTTPProxyURL, _       = url.Parse(APIBaseURL + EndpointProxyHTTP)
	HTTPProxyURLPrefixLen = len(APIEndpointBase + EndpointProxyHTTP)
)

// TestAgentConfig is a helper function to create an AgentConfig for testing purposes.
// Not used in production.
func TestAgentConfig(name string, addr string) *AgentConfig {
	return &AgentConfig{
		name: name,
		Addr: addr,
	}
}

func IsDockerHostAgent(dockerHost string) bool {
	return strings.HasPrefix(dockerHost, FakeDockerHostPrefix)
}

func GetAgentAddrFromDockerHost(dockerHost string) string {
	return dockerHost[FakeDockerHostPrefixLen:]
}

// Key implements pool.Object
func (cfg *AgentConfig) Key() string {
	return cfg.Addr
}

func (cfg *AgentConfig) FakeDockerHost() string {
	return FakeDockerHostPrefix + cfg.Addr
}

func (cfg *AgentConfig) Parse(addr string) error {
	cfg.Addr = addr
	return nil
}

func (cfg *AgentConfig) InitWithCerts(ctx context.Context, ca, crt, key []byte) error {
	clientCert, err := tls.X509KeyPair(crt, key)
	if err != nil {
		return err
	}

	// create tls config
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(ca)
	if !ok {
		return gperr.New("invalid ca certificate")
	}

	cfg.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
		ServerName:   CertsDNSName,
	}

	// create transport and http client
	cfg.httpClient = cfg.NewHTTPClient()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// check agent version
	version, _, err := cfg.Fetch(ctx, EndpointVersion)
	if err != nil {
		return err
	}

	agentVer := pkg.ParseVersion(string(version))
	serverVer := pkg.GetVersion()
	if !agentVer.IsEqual(serverVer) {
		return gperr.Errorf("agent version mismatch: server: %s, agent: %s", serverVer, agentVer)
	}

	// get agent name
	name, _, err := cfg.Fetch(ctx, EndpointName)
	if err != nil {
		return err
	}

	cfg.name = string(name)
	return nil
}

func (cfg *AgentConfig) Init(ctx context.Context) gperr.Error {
	filepath, ok := certs.AgentCertsFilepath(cfg.Addr)
	if !ok {
		return gperr.New("invalid agent host").Subject(cfg.Addr)
	}

	certData, err := os.ReadFile(filepath)
	if err != nil {
		return gperr.Wrap(err, "failed to read agent certs")
	}

	ca, crt, key, err := certs.ExtractCert(certData)
	if err != nil {
		return gperr.Wrap(err, "failed to extract agent certs")
	}

	return gperr.Wrap(cfg.InitWithCerts(ctx, ca, crt, key))
}

func (cfg *AgentConfig) NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: cfg.Transport(),
	}
}

func (cfg *AgentConfig) Transport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr != AgentHost+":443" {
				return nil, &net.AddrError{Err: "invalid address", Addr: addr}
			}
			if network != "tcp" {
				return nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil}
			}
			return cfg.DialContext(ctx)
		},
		TLSClientConfig: cfg.tlsConfig,
	}
}

func (cfg *AgentConfig) DialContext(ctx context.Context) (net.Conn, error) {
	return gphttp.DefaultDialer.DialContext(ctx, "tcp", cfg.Addr)
}

func (cfg *AgentConfig) IsInitialized() bool {
	return cfg.name != ""
}

func (cfg *AgentConfig) Name() string {
	return cfg.name
}

func (cfg *AgentConfig) String() string {
	return cfg.name + "@" + cfg.Addr
}

// MarshalMap implements pool.Object
func (cfg *AgentConfig) MarshalMap() map[string]any {
	return map[string]any{
		"name": cfg.Name(),
		"addr": cfg.Addr,
	}
}

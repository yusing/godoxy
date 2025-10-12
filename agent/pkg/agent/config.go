package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/agent/pkg/certs"
	"github.com/yusing/goutils/version"
)

type AgentConfig struct {
	Addr    string           `json:"addr"`
	Name    string           `json:"name"`
	Version version.Version  `json:"version"`
	Runtime ContainerRuntime `json:"runtime"`

	httpClient            *http.Client
	httpClientHealthCheck *http.Client
	tlsConfig             *tls.Config
	l                     zerolog.Logger
} // @name Agent

const (
	EndpointVersion    = "/version"
	EndpointName       = "/name"
	EndpointRuntime    = "/runtime"
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

func mustParseURL(urlStr string) *url.URL {
	u, err := url.Parse(urlStr)
	if err != nil {
		panic(err)
	}
	return u
}

var (
	AgentURL              = mustParseURL(APIBaseURL)
	HTTPProxyURL          = mustParseURL(APIBaseURL + EndpointProxyHTTP)
	HTTPProxyURLPrefixLen = len(APIEndpointBase + EndpointProxyHTTP)
)

func IsDockerHostAgent(dockerHost string) bool {
	return strings.HasPrefix(dockerHost, FakeDockerHostPrefix)
}

func GetAgentAddrFromDockerHost(dockerHost string) string {
	return dockerHost[FakeDockerHostPrefixLen:]
}

func (cfg *AgentConfig) FakeDockerHost() string {
	return FakeDockerHostPrefix + cfg.Addr
}

func (cfg *AgentConfig) Parse(addr string) error {
	cfg.Addr = addr
	return nil
}

var serverVersion = version.Get()

func (cfg *AgentConfig) StartWithCerts(ctx context.Context, ca, crt, key []byte) error {
	clientCert, err := tls.X509KeyPair(crt, key)
	if err != nil {
		return err
	}

	// create tls config
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(ca)
	if !ok {
		return errors.New("invalid ca certificate")
	}

	cfg.tlsConfig = tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
		ServerName:   CertsDNSName,
	}

	// create transport and http client
	cfg.httpClient = cfg.NewHTTPClient()
	applyNormalTransportConfig(cfg.httpClient)

	cfg.httpClientHealthCheck = cfg.NewHTTPClient()
	applyHealthCheckTransportConfig(cfg.httpClientHealthCheck)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// get agent name
	name, _, err := cfg.fetchString(ctx, EndpointName)
	if err != nil {
		return err
	}

	cfg.Name = name

	cfg.l = log.With().Str("agent", cfg.Name).Logger()

	// check agent version
	agentVersion, _, err := cfg.fetchString(ctx, EndpointVersion)
	if err != nil {
		return err
	}

	// check agent runtime
	runtime, status, err := cfg.fetchString(ctx, EndpointRuntime)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
		switch runtime {
		case "docker":
			cfg.Runtime = ContainerRuntimeDocker
		// case "nerdctl":
		// 	cfg.Runtime = ContainerRuntimeNerdctl
		case "podman":
			cfg.Runtime = ContainerRuntimePodman
		default:
			return fmt.Errorf("invalid agent runtime: %s", runtime)
		}
	case http.StatusNotFound:
		// backward compatibility, old agent does not have runtime endpoint
		cfg.Runtime = ContainerRuntimeDocker
	default:
		return fmt.Errorf("failed to get agent runtime: HTTP %d %s", status, runtime)
	}

	cfg.Version = version.Parse(agentVersion)

	if serverVersion.IsNewerThanMajor(cfg.Version) {
		log.Warn().Msgf("agent %s major version mismatch: server: %s, agent: %s", cfg.Name, serverVersion, cfg.Version)
	}

	log.Info().Msgf("agent %q initialized", cfg.Name)
	return nil
}

func (cfg *AgentConfig) Start(ctx context.Context) error {
	filepath, ok := certs.AgentCertsFilepath(cfg.Addr)
	if !ok {
		return fmt.Errorf("invalid agent host: %s", cfg.Addr)
	}

	certData, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read agent certs: %w", err)
	}

	ca, crt, key, err := certs.ExtractCert(certData)
	if err != nil {
		return fmt.Errorf("failed to extract agent certs: %w", err)
	}

	return cfg.StartWithCerts(ctx, ca, crt, key)
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
		TLSClientConfig: &cfg.tlsConfig,
	}
}

var dialer = &net.Dialer{Timeout: 5 * time.Second}

func (cfg *AgentConfig) DialContext(ctx context.Context) (net.Conn, error) {
	return dialer.DialContext(ctx, "tcp", cfg.Addr)
}

func (cfg *AgentConfig) String() string {
	return cfg.Name + "@" + cfg.Addr
}

func applyNormalTransportConfig(client *http.Client) {
	transport := client.Transport.(*http.Transport)
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 100
	transport.ReadBufferSize = 16384
	transport.WriteBufferSize = 16384
}

func applyHealthCheckTransportConfig(client *http.Client) {
	transport := client.Transport.(*http.Transport)
	transport.DisableKeepAlives = true
	transport.DisableCompression = true
	transport.MaxIdleConns = 1
	transport.MaxIdleConnsPerHost = 1
	transport.ReadBufferSize = 1024
	transport.WriteBufferSize = 1024
}

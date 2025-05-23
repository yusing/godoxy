package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
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
	"github.com/yusing/go-proxy/agent/pkg/certs"
	"github.com/yusing/go-proxy/pkg"
)

type AgentConfig struct {
	Addr string

	httpClient *http.Client
	tlsConfig  *tls.Config
	name       string
	version    string
	l          zerolog.Logger
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

var serverVersion = pkg.GetVersion()

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

	cfg.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
		ServerName:   CertsDNSName,
	}

	// create transport and http client
	cfg.httpClient = cfg.NewHTTPClient()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// get agent name
	name, _, err := cfg.Fetch(ctx, EndpointName)
	if err != nil {
		return err
	}

	cfg.name = string(name)

	cfg.l = log.With().Str("agent", cfg.name).Logger()

	// check agent version
	agentVersionBytes, _, err := cfg.Fetch(ctx, EndpointVersion)
	if err != nil {
		return err
	}

	cfg.version = string(agentVersionBytes)
	agentVersion := pkg.ParseVersion(cfg.version)

	if serverVersion.IsNewerMajorThan(agentVersion) {
		log.Warn().Msgf("agent %s major version mismatch: server: %s, agent: %s", cfg.name, serverVersion, agentVersion)
	}

	log.Info().Msgf("agent %q initialized", cfg.name)
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
		TLSClientConfig: cfg.tlsConfig,
	}
}

var dialer = &net.Dialer{Timeout: 5 * time.Second}

func (cfg *AgentConfig) DialContext(ctx context.Context) (net.Conn, error) {
	return dialer.DialContext(ctx, "tcp", cfg.Addr)
}

func (cfg *AgentConfig) Name() string {
	return cfg.name
}

func (cfg *AgentConfig) String() string {
	return cfg.name + "@" + cfg.Addr
}

func (cfg *AgentConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{
		"name":    cfg.Name(),
		"addr":    cfg.Addr,
		"version": cfg.version,
	})
}

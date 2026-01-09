package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/agent/pkg/agent/common"
	agentstream "github.com/yusing/godoxy/agent/pkg/agent/stream"
	"github.com/yusing/godoxy/agent/pkg/certs"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/version"
)

type AgentConfig struct {
	AgentInfo

	Addr                 string `json:"addr"`
	IsTCPStreamSupported bool   `json:"supports_tcp_stream"`
	IsUDPStreamSupported bool   `json:"supports_udp_stream"`

	// for stream
	caCert     *x509.Certificate
	clientCert *tls.Certificate

	tlsConfig tls.Config

	l zerolog.Logger
} // @name Agent

type AgentInfo struct {
	Version version.Version  `json:"version" swaggertype:"string"`
	Name    string           `json:"name"`
	Runtime ContainerRuntime `json:"runtime"`
}

// Deprecated. Replaced by EndpointInfo
const (
	EndpointVersion = "/version"
	EndpointName    = "/name"
	EndpointRuntime = "/runtime"
)

const (
	EndpointInfo       = "/info"
	EndpointProxyHTTP  = "/proxy/http"
	EndpointHealth     = "/health"
	EndpointLogs       = "/logs"
	EndpointSystemInfo = "/system_info"

	AgentHost = common.CertsDNSName

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

// InitWithCerts initializes the agent config with the given CA, certificate, and key.
func (cfg *AgentConfig) InitWithCerts(ctx context.Context, ca, crt, key []byte) error {
	clientCert, err := tls.X509KeyPair(crt, key)
	if err != nil {
		return err
	}
	cfg.clientCert = &clientCert

	// create tls config
	caCertPool := x509.NewCertPool()
	ok := caCertPool.AppendCertsFromPEM(ca)
	if !ok {
		return errors.New("invalid ca certificate")
	}
	// Keep the CA leaf for stream client dialing.
	if block, _ := pem.Decode(ca); block == nil || block.Type != "CERTIFICATE" {
		return errors.New("invalid ca certificate")
	} else if cert, err := x509.ParseCertificate(block.Bytes); err != nil {
		return err
	} else {
		cfg.caCert = cert
	}

	cfg.tlsConfig = tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
		ServerName:   common.CertsDNSName,
		MinVersion:   tls.VersionTLS12,
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	status, err := cfg.fetchJSON(ctx, EndpointInfo, &cfg.AgentInfo)
	if err != nil {
		return err
	}

	var streamUnsupportedErrs gperr.Builder

	if status == http.StatusOK {
		// test stream server connection
		const fakeAddress = "localhost:8080" // it won't be used, just for testing
		// test TCP stream support
		err := agentstream.TCPHealthCheck(cfg.Addr, cfg.caCert, cfg.clientCert)
		if err != nil {
			streamUnsupportedErrs.Addf("failed to connect to stream server via TCP: %w", err)
		} else {
			cfg.IsTCPStreamSupported = true
		}

		// test UDP stream support
		err = agentstream.UDPHealthCheck(cfg.Addr, cfg.caCert, cfg.clientCert)
		if err != nil {
			streamUnsupportedErrs.Addf("failed to connect to stream server via UDP: %w", err)
		} else {
			cfg.IsUDPStreamSupported = true
		}
	} else {
		// old agent does not support EndpointInfo
		// fallback with old logic
		cfg.IsTCPStreamSupported = false
		cfg.IsUDPStreamSupported = false
		streamUnsupportedErrs.Adds("agent version is too old, does not support stream tunneling")

		// get agent name
		name, _, err := cfg.fetchString(ctx, EndpointName)
		if err != nil {
			return err
		}

		cfg.Name = name

		// check agent version
		agentVersion, _, err := cfg.fetchString(ctx, EndpointVersion)
		if err != nil {
			return err
		}

		cfg.Version = version.Parse(agentVersion)

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
	}

	cfg.l = log.With().Str("agent", cfg.Name).Logger()

	if err := streamUnsupportedErrs.Error(); err != nil {
		gperr.LogWarn("agent has limited/no stream tunneling support, TCP and UDP routes via agent will not work", err, &cfg.l)
	}

	if serverVersion.IsNewerThanMajor(cfg.Version) {
		log.Warn().Msgf("agent %s major version mismatch: server: %s, agent: %s", cfg.Name, serverVersion, cfg.Version)
	}

	log.Info().Msgf("agent %q initialized", cfg.Name)
	return nil
}

// Init initializes the agent config with the given context.
func (cfg *AgentConfig) Init(ctx context.Context) error {
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

	return cfg.InitWithCerts(ctx, ca, crt, key)
}

// NewTCPClient creates a new TCP client for the agent.
//
// It returns an error if
//   - the agent is not initialized
//   - the agent does not support TCP stream tunneling
//   - the agent stream server address is not initialized
func (cfg *AgentConfig) NewTCPClient(targetAddress string) (net.Conn, error) {
	if cfg.caCert == nil || cfg.clientCert == nil {
		return nil, errors.New("agent is not initialized")
	}
	if !cfg.IsTCPStreamSupported {
		return nil, errors.New("agent does not support TCP stream tunneling")
	}
	return agentstream.NewTCPClient(cfg.Addr, targetAddress, cfg.caCert, cfg.clientCert)
}

// NewUDPClient creates a new UDP client for the agent.
//
// It returns an error if
//   - the agent is not initialized
//   - the agent does not support UDP stream tunneling
//   - the agent stream server address is not initialized
func (cfg *AgentConfig) NewUDPClient(targetAddress string) (net.Conn, error) {
	if cfg.caCert == nil || cfg.clientCert == nil {
		return nil, errors.New("agent is not initialized")
	}
	if !cfg.IsUDPStreamSupported {
		return nil, errors.New("agent does not support UDP stream tunneling")
	}
	return agentstream.NewUDPClient(cfg.Addr, targetAddress, cfg.caCert, cfg.clientCert)
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

func (cfg *AgentConfig) TLSConfig() *tls.Config {
	return &cfg.tlsConfig
}

var dialer = &net.Dialer{Timeout: 5 * time.Second}

func (cfg *AgentConfig) DialContext(ctx context.Context) (net.Conn, error) {
	return dialer.DialContext(ctx, "tcp", cfg.Addr)
}

func (cfg *AgentConfig) String() string {
	return cfg.Name + "@" + cfg.Addr
}

func (cfg *AgentConfig) do(ctx context.Context, method, endpoint string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, APIBaseURL+endpoint, body)
	if err != nil {
		return nil, err
	}
	client := http.Client{
		Transport: cfg.Transport(),
	}
	return client.Do(req)
}

func (cfg *AgentConfig) fetchString(ctx context.Context, endpoint string) (string, int, error) {
	resp, err := cfg.do(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	data, release, err := httputils.ReadAllBody(resp)
	if err != nil {
		return "", 0, err
	}
	ret := string(data)
	release(data)
	return ret, resp.StatusCode, nil
}

// fetchJSON fetches a JSON response from the agent and unmarshals it into the provided struct
//
// It will return the status code of the response, and error if any.
// If the status code is not http.StatusOK, out will be unchanged but error will still be nil.
func (cfg *AgentConfig) fetchJSON(ctx context.Context, endpoint string, out any) (int, error) {
	resp, err := cfg.do(ctx, "GET", endpoint, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	data, release, err := httputils.ReadAllBody(resp)
	if err != nil {
		return 0, err
	}

	defer release(data)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}

	err = json.Unmarshal(data, out)
	if err != nil {
		return 0, err
	}
	return resp.StatusCode, nil
}

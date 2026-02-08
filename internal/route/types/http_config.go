package route

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	gperr "github.com/yusing/goutils/errs"
)

type HTTPConfig struct {
	NoTLSVerify           bool          `json:"no_tls_verify,omitempty"`
	ResponseHeaderTimeout time.Duration `json:"response_header_timeout,omitempty" swaggertype:"primitive,integer"`
	DisableCompression    bool          `json:"disable_compression,omitempty"`

	// SSL/TLS proxy options (nginx-like)
	SSLServerName         *string  `json:"ssl_server_name,omitempty"`         // SNI server name
	SSLTrustedCertificate string   `json:"ssl_trusted_certificate,omitempty"` // Path to trusted CA certificates
	SSLCertificate        string   `json:"ssl_certificate,omitempty"`         // Path to client certificate
	SSLCertificateKey     string   `json:"ssl_certificate_key,omitempty"`     // Path to client certificate key
	SSLProtocols          []string `json:"ssl_protocols,omitempty"`           // Allowed TLS protocols
}

// BuildTLSConfig creates a TLS configuration based on the HTTP config options.
func (cfg *HTTPConfig) BuildTLSConfig(targetURL *url.URL) (*tls.Config, error) {
	tlsConfig := &tls.Config{}

	// Handle InsecureSkipVerify (legacy NoTLSVerify option)
	if cfg.NoTLSVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	// Handle ssl_server_name (SNI)
	if cfg.SSLServerName != nil {
		switch *cfg.SSLServerName {
		case "off":
			// Disable SNI by setting empty string
			tlsConfig.ServerName = ""
		case "on", "":
			// Use hostname from target URL for SNI
			tlsConfig.ServerName = targetURL.Hostname()
		default:
			tlsConfig.ServerName = *cfg.SSLServerName
		}
	} else {
		// Default behavior - use hostname for SNI
		tlsConfig.ServerName = targetURL.Hostname()
	}

	// Handle ssl_trusted_certificate
	if cfg.SSLTrustedCertificate != "" {
		caCertData, err := os.ReadFile(cfg.SSLTrustedCertificate)
		if err != nil {
			return nil, gperr.PrependSubject(err, cfg.SSLTrustedCertificate)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCertData) {
			return nil, gperr.PrependSubject(errors.New("failed to parse trusted certificates"), cfg.SSLTrustedCertificate)
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Handle ssl_certificate and ssl_certificate_key (client certificates)
	if cfg.SSLCertificate != "" {
		if cfg.SSLCertificateKey == "" {
			return nil, errors.New("ssl_certificate_key is required when ssl_certificate is specified")
		}

		clientCert, err := tls.LoadX509KeyPair(cfg.SSLCertificate, cfg.SSLCertificateKey)
		if err != nil {
			return nil, gperr.PrependSubject(err, cfg.SSLCertificate)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	} else if cfg.SSLCertificateKey != "" {
		return nil, errors.New("ssl_certificate is required when ssl_certificate_key is specified")
	}

	// Handle ssl_protocols (TLS versions)
	if len(cfg.SSLProtocols) > 0 {
		var minVersion, maxVersion uint16

		for _, protocol := range cfg.SSLProtocols {
			var version uint16
			switch strings.ToLower(protocol) {
			case "tlsv1.0":
				version = tls.VersionTLS10
			case "tlsv1.1":
				version = tls.VersionTLS11
			case "tlsv1.2":
				version = tls.VersionTLS12
			case "tlsv1.3":
				version = tls.VersionTLS13
			default:
				return nil, gperr.New("unsupported TLS protocol").
					Subject(protocol)
			}

			if minVersion == 0 || version < minVersion {
				minVersion = version
			}
			if maxVersion == 0 || version > maxVersion {
				maxVersion = version
			}
		}

		tlsConfig.MinVersion = minVersion
		tlsConfig.MaxVersion = maxVersion
	}

	return tlsConfig, nil
}

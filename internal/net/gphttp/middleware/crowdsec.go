package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yusing/godoxy/internal/route/routes"
	httputils "github.com/yusing/goutils/http"
	ioutils "github.com/yusing/goutils/io"
)

type (
	crowdsecMiddleware struct {
		CrowdsecMiddlewareOpts
	}

	CrowdsecMiddlewareOpts struct {
		Route      string        `json:"route" validate:"required"`   // route name (alias) or IP address
		Port       int           `json:"port"`                        // port number (optional if using route name)
		APIKey     string        `json:"api_key" validate:"required"` // API key for CrowdSec AppSec (mandatory)
		Endpoint   string        `json:"endpoint"`                    // default: "/"
		LogBlocked bool          `json:"log_blocked"`                 // default: false
		Timeout    time.Duration `json:"timeout"`                     // default: 5 seconds

		httpClient *http.Client
	}
)

var Crowdsec = NewMiddleware[crowdsecMiddleware]()

func (m *crowdsecMiddleware) setup() {
	m.CrowdsecMiddlewareOpts = CrowdsecMiddlewareOpts{
		Route:      "",
		Port:       7422, // default port for CrowdSec AppSec
		APIKey:     "",
		Endpoint:   "/",
		LogBlocked: false,
		Timeout:    5 * time.Second,
	}
}

func (m *crowdsecMiddleware) finalize() error {
	if !strings.HasPrefix(m.Endpoint, "/") {
		return fmt.Errorf("endpoint must start with /")
	}
	if m.Timeout == 0 {
		m.Timeout = 5 * time.Second
	}
	m.httpClient = &http.Client{
		Timeout: m.Timeout,
		// do not follow redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return nil
}

// before implements RequestModifier.
func (m *crowdsecMiddleware) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	// Build CrowdSec URL
	crowdsecURL, err := m.buildCrowdSecURL()
	if err != nil {
		Crowdsec.LogError(r).Err(err).Msg("failed to build CrowdSec URL")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}

	// Determine HTTP method: GET for requests without body, POST for requests with body
	method := http.MethodGet
	var body io.Reader
	if r.Body != nil && r.Body != http.NoBody {
		method = http.MethodPost
		// Read the body
		bodyBytes, release, err := httputils.ReadAllRequestBody(r)
		if err != nil {
			Crowdsec.LogError(r).Err(err).Msg("failed to read request body")
			w.WriteHeader(http.StatusInternalServerError)
			return false
		}
		r.Body = ioutils.NewHookReadCloser(io.NopCloser(bytes.NewReader(bodyBytes)), func() {
			release(bodyBytes)
		})
		body = bytes.NewReader(bodyBytes)
	}

	ctx, cancel := context.WithTimeout(r.Context(), m.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, crowdsecURL, body)
	if err != nil {
		Crowdsec.LogError(r).Err(err).Msg("failed to create CrowdSec request")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}

	// Get remote IP
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}

	// Get HTTP version in integer form (10, 11, 20, etc.)
	httpVersion := m.getHTTPVersion(r)

	// Copy original headers
	req.Header = r.Header.Clone()

	// Overwrite CrowdSec required headers to prevent spoofing
	req.Header.Set("X-Crowdsec-Appsec-Ip", remoteIP)
	req.Header.Set("X-Crowdsec-Appsec-Uri", r.URL.RequestURI())
	req.Header.Set("X-Crowdsec-Appsec-Host", r.Host)
	req.Header.Set("X-Crowdsec-Appsec-Verb", r.Method)
	req.Header.Set("X-Crowdsec-Appsec-Api-Key", m.APIKey)
	req.Header.Set("X-Crowdsec-Appsec-User-Agent", r.UserAgent())
	req.Header.Set("X-Crowdsec-Appsec-Http-Version", httpVersion)

	// Make request to CrowdSec
	resp, err := m.httpClient.Do(req)
	if err != nil {
		Crowdsec.LogError(r).Err(err).Msg("failed to connect to CrowdSec server")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}
	defer resp.Body.Close()

	// Handle response codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Request is allowed
		return true
	case http.StatusForbidden:
		// Request is blocked by CrowdSec
		if m.LogBlocked {
			Crowdsec.LogWarn(r).
				Str("ip", remoteIP).
				Msg("request blocked by CrowdSec")
		}
		w.WriteHeader(http.StatusForbidden)
		return false
	case http.StatusInternalServerError:
		// CrowdSec server error
		bodyBytes, release, err := httputils.ReadAllBody(resp)
		if err == nil {
			defer release(bodyBytes)
			Crowdsec.LogError(r).
				Str("crowdsec_response", string(bodyBytes)).
				Msg("CrowdSec server error")
		}
		w.WriteHeader(http.StatusInternalServerError)
		return false
	default:
		// Unexpected response code
		Crowdsec.LogWarn(r).
			Int("status_code", resp.StatusCode).
			Msg("unexpected response from CrowdSec server")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}
}

// buildCrowdSecURL constructs the CrowdSec server URL based on route or IP configuration
func (m *crowdsecMiddleware) buildCrowdSecURL() (string, error) {
	// Try to get route first
	if m.Route != "" {
		if route, ok := routes.HTTP.Get(m.Route); ok {
			// Using route name
			targetURL := *route.TargetURL()
			targetURL.Path = m.Endpoint
			return targetURL.String(), nil
		}

		// If not found in routes, assume it's an IP address
		if m.Port == 0 {
			return "", fmt.Errorf("port must be specified when using IP address")
		}
		return fmt.Sprintf("http://%s%s", net.JoinHostPort(m.Route, strconv.Itoa(m.Port)), m.Endpoint), nil
	}

	return "", fmt.Errorf("route or IP address must be specified")
}

func (m *crowdsecMiddleware) getHTTPVersion(r *http.Request) string {
	switch {
	case r.ProtoMajor == 1 && r.ProtoMinor == 0:
		return "10"
	case r.ProtoMajor == 1 && r.ProtoMinor == 1:
		return "11"
	case r.ProtoMajor == 2:
		return "20"
	case r.ProtoMajor == 3:
		return "30"
	default:
		return strconv.Itoa(r.ProtoMajor*10 + r.ProtoMinor)
	}
}

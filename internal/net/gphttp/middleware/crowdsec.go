package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/yusing/godoxy/internal/route/routes"
	httputils "github.com/yusing/goutils/http"
)

type (
	crowdsecMiddleware struct {
		CrowdsecMiddlewareOpts
	}

	CrowdsecMiddlewareOpts struct {
		Route      string `json:"route" validate:"required"`   // route name (alias) or IP address
		Port       int    `json:"port"`                        // port number (optional if using route name)
		APIKey     string `json:"api_key" validate:"required"` // API key for CrowdSec AppSec (mandatory)
		Endpoint   string `json:"endpoint"`                    // default: "/"
		httpClient *http.Client
	}
)

var Crowdsec = NewMiddleware[crowdsecMiddleware]()

func (m *crowdsecMiddleware) setup() {
	m.CrowdsecMiddlewareOpts = CrowdsecMiddlewareOpts{
		Route:    "",
		Port:     0,
		APIKey:   "",
		Endpoint: "/",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			// do not follow redirects
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// before implements RequestModifier.
func (m *crowdsecMiddleware) before(w http.ResponseWriter, r *http.Request) (proceed bool) {
	// Validate API key is configured
	if m.APIKey == "" {
		Crowdsec.LogError(r).Msg("API key is required for CrowdSec middleware")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}

	// Build CrowdSec URL
	crowdsecURL, err := m.buildCrowdSecURL()
	if err != nil {
		Crowdsec.LogError(r).Err(err).Msg("failed to build CrowdSec URL")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}

	Crowdsec.LogWarn(r).Str("crowdsec_url: ", crowdsecURL).Msg("crowdsec url")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Determine HTTP method: GET for requests without body, POST for requests with body
	method := http.MethodGet
	var body io.Reader
	if r.Body != nil && r.ContentLength > 0 {
		method = http.MethodPost
		// Read the body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			Crowdsec.LogError(r).Err(err).Msg("failed to read request body")
			w.WriteHeader(http.StatusInternalServerError)
			return false
		}
		// Restore the body for downstream processing
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		body = bytes.NewBuffer(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, crowdsecURL, body)
	if err != nil {
		Crowdsec.LogError(r).Err(err).Msg("failed to create CrowdSec request")
		w.WriteHeader(http.StatusInternalServerError)
		return false
	}

	// Get real IP
	realIP := m.getRealIP(r)

	// Get HTTP version in integer form (10, 11, 20, etc.)
	httpVersion := m.getHTTPVersion(r)

	// Set CrowdSec required headers
	req.Header.Set("X-Crowdsec-Appsec-Ip", realIP)
	req.Header.Set("X-Crowdsec-Appsec-Uri", r.URL.RequestURI())
	req.Header.Set("X-Crowdsec-Appsec-Host", r.Host)
	req.Header.Set("X-Crowdsec-Appsec-Verb", r.Method)
	req.Header.Set("X-Crowdsec-Appsec-Api-Key", m.APIKey)
	req.Header.Set("X-Crowdsec-Appsec-User-Agent", r.UserAgent())
	req.Header.Set("X-Crowdsec-Appsec-Http-Version", httpVersion)

	// Copy original headers
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

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
		Crowdsec.LogWarn(r).
			Str("ip", realIP).
			Str("uri", r.URL.RequestURI()).
			Msg("request blocked by CrowdSec")
		w.WriteHeader(http.StatusForbidden)
		return false
	case http.StatusInternalServerError:
		// CrowdSec server error
		bodyBytes, release, _ := httputils.ReadAllBody(resp)
		defer release(bodyBytes)
		Crowdsec.LogError(r).
			Str("crowdsec_response", string(bodyBytes)).
			Msg("CrowdSec server error")
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
		return fmt.Sprintf("http://%s:%d%s", m.Route, m.Port, m.Endpoint), nil
	}

	return "", fmt.Errorf("route or IP address must be specified")
}

// getRealIP extracts the real client IP from the request
// Note: If real_ip middleware is configured in the chain, r.RemoteAddr will already contain the real IP
func (m *crowdsecMiddleware) getRealIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		Crowdsec.LogWarn(r).Str("remote_addr", r.RemoteAddr).Msg("remote ip request")
		return r.RemoteAddr
	}
	return ip
}

// getHTTPVersion returns the HTTP version in integer form (10, 11, 20, etc.)
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

package routeApi

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
	"github.com/yusing/godoxy/internal/common"
	config "github.com/yusing/godoxy/internal/config/types"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/route/rules"
	apitypes "github.com/yusing/goutils/apitypes"
	gperr "github.com/yusing/goutils/errs"
	httputils "github.com/yusing/goutils/http"
)

type RawRule struct {
	Name string `json:"name"`
	On   string `json:"on"`
	Do   string `json:"do"`
}

type PlaygroundRequest struct {
	Rules        string       `json:"rules" binding:"required"`
	MockRequest  MockRequest  `json:"mockRequest"`
	MockResponse MockResponse `json:"mockResponse"`
} // @name PlaygroundRequest

type MockRequest struct {
	Method   string              `json:"method"`
	Path     string              `json:"path"`
	Host     string              `json:"host"`
	Headers  map[string][]string `json:"headers"`
	Query    map[string][]string `json:"query"`
	Cookies  []MockCookie        `json:"cookies"`
	Body     string              `json:"body"`
	RemoteIP string              `json:"remoteIP"`
} // @name MockRequest

type MockCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
} // @name MockCookie

type MockResponse struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
} // @name MockResponse

type PlaygroundResponse struct {
	ParsedRules    []ParsedRule  `json:"parsedRules"`
	MatchedRules   []string      `json:"matchedRules"`
	FinalRequest   FinalRequest  `json:"finalRequest"`
	FinalResponse  FinalResponse `json:"finalResponse"`
	ExecutionError error         `json:"executionError,omitempty"` // we need the structured error, not the plain string
	UpstreamCalled bool          `json:"upstreamCalled"`
} // @name PlaygroundResponse

type ParsedRule struct {
	Name            string `json:"name"`
	On              string `json:"on"`
	Do              string `json:"do"`
	ValidationError error  `json:"validationError,omitempty"` // we need the structured error, not the plain string
} // @name ParsedRule

type FinalRequest struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Host    string              `json:"host"`
	Headers map[string][]string `json:"headers"`
	Query   map[string][]string `json:"query"`
	Body    string              `json:"body"`
} // @name FinalRequest

type FinalResponse struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
} // @name FinalResponse

// @x-id				"playground"
// @BasePath		/api/v1
// @Summary		Rule Playground
// @Description	Test rules against mock request/response
// @Tags			route
// @Accept			json
// @Produce		json
// @Param			request	body		PlaygroundRequest	true	"Playground request"
// @Success		200		{object}	PlaygroundResponse
// @Failure		400		{object}	apitypes.ErrorResponse
// @Failure		403		{object}	apitypes.ErrorResponse
// @Router			/route/playground [post]
func Playground(c *gin.Context) {
	var req PlaygroundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	// Apply defaults
	if req.MockRequest.Method == "" {
		req.MockRequest.Method = "GET"
	}
	if req.MockRequest.Path == "" {
		req.MockRequest.Path = "/"
	}
	if req.MockRequest.Host == "" {
		req.MockRequest.Host = "localhost"
	}

	// Create mock HTTP request
	mockReq, err := createMockRequest(c.Request.Context(), req.MockRequest)
	if err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid mock request", err))
		return
	}
	if status := req.MockResponse.StatusCode; status != 0 && (status < 100 || status > 999) {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid mock response status", gperr.New("status must be 0 or between 100 and 999")))
		return
	}

	// Parse rules only after checking the inputs, before any action can run.
	parsedRules, rulesList, parseErr := parseRules(req.Rules)
	if parseErr == nil {
		parseErr = nettypes.ValidateAddressSetReferences(rulesList, config.AddressSetCheck(c.Request.Context()))
		for i := range parsedRules {
			parsedRules[i].ValidationError = parseErr
		}
	}

	// Create mock HTTP response writer
	recorder := httptest.NewRecorder()

	// Execute rules
	matchedRules := []string{}
	upstreamCalled := false
	var executionError error

	// Capture the request at the mock upstream, before its body is consumed.
	var finalRequest FinalRequest
	captureRequest := func(r *http.Request) {
		var body []byte
		if r.Body != nil {
			var err error
			body, err = io.ReadAll(r.Body)
			if err != nil {
				executionError = gperr.Join(executionError, err)
			}
			r.Body.Close()
			r.Body = io.NopCloser(strings.NewReader(string(body)))
		}
		finalRequest = FinalRequest{
			Method: r.Method, Path: r.URL.Path, Host: r.Host,
			Headers: r.Header.Clone(), Query: r.URL.Query(), Body: string(body),
		}
		if r.URL.RawPath != "" {
			finalRequest.Path = r.URL.RawPath
		}
	}

	if parseErr == nil && len(rulesList) > 0 {
		// Create upstream handler that records if it was called and captures request state
		upstreamHandler := func(w http.ResponseWriter, r *http.Request) {
			upstreamCalled = true
			captureRequest(r)
			for k, values := range req.MockResponse.Headers {
				for _, v := range values {
					w.Header().Add(k, v)
				}
			}

			// If there's mock response body, write it during upstream call
			if req.MockResponse.Body != "" && w.Header().Get("Content-Type") == "" {
				w.Header().Set("Content-Type", "text/plain")
			}
			if req.MockResponse.StatusCode > 0 {
				w.WriteHeader(req.MockResponse.StatusCode)
			}
			if req.MockResponse.Body != "" && req.MockRequest.Method != http.MethodHead &&
				(req.MockResponse.StatusCode == 0 || req.MockResponse.StatusCode >= 200) &&
				req.MockResponse.StatusCode != http.StatusNoContent && req.MockResponse.StatusCode != http.StatusNotModified {
				w.Write([]byte(req.MockResponse.Body))
			}
		}

		// Build handler with rules
		handler := rulesList.BuildHandler(upstreamHandler)

		// Execute the handler
		handlerWithRecover(recorder, mockReq, handler, &executionError)

		// Track which rules matched
		// Since we can't easily instrument the rules, we'll check each rule manually
		matchedRules = checkMatchedRules(rulesList, recorder, mockReq)
	} else if parseErr != nil {
		executionError = parseErr
	}

	if !upstreamCalled {
		captureRequest(mockReq)
	}

	// Build final response state
	finalResponse := FinalResponse{
		StatusCode: recorder.Code,
		Headers:    recorder.Header(),
		Body:       recorder.Body.String(),
	}

	// Ensure status code defaults to 200 if not set
	if finalResponse.StatusCode == 0 {
		finalResponse.StatusCode = http.StatusOK
	}

	// prevent null in response
	if parsedRules == nil {
		parsedRules = []ParsedRule{}
	}
	if matchedRules == nil {
		matchedRules = []string{}
	}

	response := PlaygroundResponse{
		ParsedRules:    parsedRules,
		MatchedRules:   matchedRules,
		FinalRequest:   finalRequest,
		FinalResponse:  finalResponse,
		ExecutionError: executionError,
		UpstreamCalled: upstreamCalled,
	}

	if common.IsTest {
		c.Set("response", response)
	}
	c.JSON(http.StatusOK, response)
}

func handlerWithRecover(w http.ResponseWriter, r *http.Request, h http.HandlerFunc, outErr *error) {
	defer func() {
		if r := recover(); r != nil {
			if outErr != nil {
				*outErr = gperr.Errorf("panic during rule execution: %v", r)
			}
		}
	}()
	h(w, r)
}

func parseRules(config string) ([]ParsedRule, rules.Rules, error) {
	config = strings.TrimSpace(config)
	if config == "" {
		return []ParsedRule{}, nil, nil
	}

	var rulesList rules.Rules
	if err := rulesList.Parse(config); err != nil {
		// Keep per-rule YAML diagnostics, but use the shared parser for execution
		// so interpolation and validation agree with route configuration.
		var rawRules []RawRule
		if yaml.Unmarshal([]byte(config), &rawRules) == nil && len(rawRules) > 0 {
			return parseRawRules(rawRules), nil, err
		}
		return nil, nil, err
	}
	validationErr := rulesList.Validate()

	parsedRules := make([]ParsedRule, 0, len(rulesList))
	for _, rule := range rulesList {
		parsedRules = append(parsedRules, ParsedRule{
			Name: rule.Name,
			On:   rule.On.String(),
			Do:   rule.Do.String(),
			// A rejected rule set cannot execute any row. Keep the shared
			// validation explanation visible in the existing per-row results.
			ValidationError: validationErr,
		})
	}

	return parsedRules, rulesList, validationErr
}

func parseRawRules(rawRules []RawRule) []ParsedRule {
	parsedRules := make([]ParsedRule, 0, len(rawRules))

	// Parse each rule individually to capture per-rule errors
	for _, rawRule := range rawRules {
		var rule rules.Rule

		// Extract fields
		name := rawRule.Name
		onStr := rawRule.On
		doStr := rawRule.Do

		// Parse On
		var onErr error
		if onStr != "" {
			onErr = rule.On.Parse(onStr)
		}

		// Parse Do
		var doErr error
		if doStr != "" {
			doErr = rule.Do.Parse(doStr)
		}

		validationErr := gperr.Join(gperr.PrependSubject(onErr, "on"), gperr.PrependSubject(doErr, "do"))

		parsedRules = append(parsedRules, ParsedRule{
			Name:            name,
			On:              onStr,
			Do:              doStr,
			ValidationError: validationErr,
		})
	}

	return parsedRules
}

func createMockRequest(ctx context.Context, mock MockRequest) (*http.Request, error) {
	// Create request
	var body io.Reader
	if mock.Body != "" {
		body = strings.NewReader(mock.Body)
	}

	req, err := http.NewRequestWithContext(ctx, mock.Method, mock.Path, body)
	if err != nil {
		return nil, err
	}
	if len(mock.Query) > 0 {
		query := req.URL.Query()
		for name, values := range mock.Query {
			query[name] = values
		}
		req.URL.RawQuery = query.Encode()
	}
	req.RequestURI = req.URL.RequestURI()
	if req.URL.Scheme == "https" {
		req.TLS = &tls.ConnectionState{HandshakeComplete: true}
	}

	// Set host
	req.Host = mock.Host

	// Set headers
	for name, values := range mock.Headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	// Set cookies
	if mock.Cookies != nil {
		for _, cookie := range mock.Cookies {
			req.AddCookie(&http.Cookie{
				Name:  cookie.Name,
				Value: cookie.Value,
			})
		}
	}

	// Set remote address
	if mock.RemoteIP != "" {
		req.RemoteAddr = net.JoinHostPort(mock.RemoteIP, "0")
	} else {
		req.RemoteAddr = "127.0.0.1:0"
	}

	return req, nil
}

func checkMatchedRules(rulesList rules.Rules, w *httptest.ResponseRecorder, r *http.Request) []string {
	var matched []string

	// This is a final-state matcher view, not an execution trace.
	response := w.Result()
	defer response.Body.Close()
	rm := httputils.ResponseAsRW(response)

	for _, rule := range rulesList {
		// Check if rule matches
		if rule.Check(rm, r) {
			matched = append(matched, rule.Name)
		}
	}

	return matched
}

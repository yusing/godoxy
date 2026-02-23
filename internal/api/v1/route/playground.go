package routeApi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/common"
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
	Rules        []RawRule    `json:"rules" binding:"required"`
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

	// Parse rules
	parsedRules, rulesList, parseErr := parseRules(req.Rules)

	// Create mock HTTP request
	mockReq := createMockRequest(req.MockRequest)

	// Create mock HTTP response writer
	recorder := httptest.NewRecorder()

	// Set initial mock response if provided
	if req.MockResponse.StatusCode > 0 {
		recorder.Code = req.MockResponse.StatusCode
	}
	if req.MockResponse.Headers != nil {
		for k, values := range req.MockResponse.Headers {
			for _, v := range values {
				recorder.Header().Add(k, v)
			}
		}
	}
	if req.MockResponse.Body != "" {
		recorder.Body.WriteString(req.MockResponse.Body)
	}

	// Execute rules
	matchedRules := []string{}
	upstreamCalled := false
	var executionError error

	// Variables to capture modified request state
	var finalReqMethod, finalReqPath, finalReqHost string
	var finalReqHeaders http.Header
	var finalReqQuery url.Values

	if parseErr == nil && len(rulesList) > 0 {
		// Create upstream handler that records if it was called and captures request state
		upstreamHandler := func(w http.ResponseWriter, r *http.Request) {
			upstreamCalled = true
			// Capture the request state when upstream is called
			finalReqMethod = r.Method
			finalReqPath = r.URL.Path
			finalReqHost = r.Host
			finalReqHeaders = r.Header.Clone()
			finalReqQuery = r.URL.Query()

			// Debug: also check RequestURI
			if r.URL.Path != r.URL.RawPath && r.URL.RawPath != "" {
				finalReqPath = r.URL.RawPath
			}

			// If there's mock response body, write it during upstream call
			if req.MockResponse.Body != "" && w.Header().Get("Content-Type") == "" {
				w.Header().Set("Content-Type", "text/plain")
			}
			if req.MockResponse.StatusCode > 0 {
				w.WriteHeader(req.MockResponse.StatusCode)
			}
			if req.MockResponse.Body != "" {
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

	// Build final request state
	// Use captured state if upstream was called, otherwise use current state
	var finalRequest FinalRequest
	if upstreamCalled {
		finalRequest = FinalRequest{
			Method:  finalReqMethod,
			Path:    finalReqPath,
			Host:    finalReqHost,
			Headers: finalReqHeaders,
			Query:   finalReqQuery,
			Body:    req.MockRequest.Body,
		}
	} else {
		finalRequest = FinalRequest{
			Method:  mockReq.Method,
			Path:    mockReq.URL.Path,
			Host:    mockReq.Host,
			Headers: mockReq.Header,
			Query:   mockReq.URL.Query(),
			Body:    req.MockRequest.Body,
		}
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
				*outErr = fmt.Errorf("panic during rule execution: %v", r)
			}
		}
	}()
	h(w, r)
}

func parseRules(rawRules []RawRule) ([]ParsedRule, rules.Rules, error) {
	parsedRules := make([]ParsedRule, 0, len(rawRules))
	rulesList := make(rules.Rules, 0, len(rawRules))

	var valErrs gperr.Builder

	// Parse each rule individually to capture per-rule errors
	for _, rawRule := range rawRules {
		var rule rules.Rule

		// Extract fields
		name := rawRule.Name
		onStr := rawRule.On
		doStr := rawRule.Do

		rule.Name = name

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

		// Determine if valid
		isValid := onErr == nil && doErr == nil
		var validationErr error
		if !isValid {
			validationErr = gperr.Join(gperr.PrependSubject(onErr, "on"), gperr.PrependSubject(doErr, "do"))
			valErrs.Add(validationErr)
		}

		parsedRules = append(parsedRules, ParsedRule{
			Name:            name,
			On:              onStr,
			Do:              doStr,
			ValidationError: validationErr,
		})

		// Only add valid rules to execution list
		if isValid {
			rulesList = append(rulesList, rule)
		}
	}

	return parsedRules, rulesList, valErrs.Error()
}

func createMockRequest(mock MockRequest) *http.Request {
	// Create URL
	urlStr := mock.Path
	if len(mock.Query) > 0 {
		query := url.Values(mock.Query)
		urlStr = mock.Path + "?" + query.Encode()
	}

	// Create request
	var body io.Reader
	if mock.Body != "" {
		body = strings.NewReader(mock.Body)
	}

	req := httptest.NewRequest(mock.Method, urlStr, body)

	// Set host
	req.Host = mock.Host

	// Set headers
	req.Header = mock.Headers

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
		req.RemoteAddr = mock.RemoteIP + ":0"
	} else {
		req.RemoteAddr = "127.0.0.1:0"
	}

	return req
}

func checkMatchedRules(rulesList rules.Rules, w http.ResponseWriter, r *http.Request) []string {
	var matched []string

	// Create a ResponseModifier to properly check rules
	rm := httputils.NewResponseModifier(w)

	for _, rule := range rulesList {
		// Check if rule matches
		if rule.Check(rm, r) {
			matched = append(matched, rule.Name)
		}
	}

	return matched
}

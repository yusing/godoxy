package httpheaders

import (
	"net/http"
	"strings"
)

// AppendCSP appends a CSP header to specific directives in the response writer.
//
// Directives other than the ones in cspDirectives will be kept as is.
//
// It will replace 'none' with the sources.
//
// It will append 'self' to the sources if it's not already present.
func AppendCSP(w http.ResponseWriter, r *http.Request, cspDirectives []string, sources []string) {
	csp := make(map[string]string)
	cspValues := r.Header.Values("Content-Security-Policy")
	if len(cspValues) == 1 {
		cspValues = strings.Split(cspValues[0], ";")
		for i, cspString := range cspValues {
			cspValues[i] = strings.TrimSpace(cspString)
		}
	}

	for _, cspString := range cspValues {
		parts := strings.SplitN(cspString, " ", 2)
		if len(parts) == 2 {
			csp[parts[0]] = parts[1]
		}
	}

	for _, directive := range cspDirectives {
		value, ok := csp[directive]
		if !ok {
			value = "'self'"
		}
		switch value {
		case "'self'":
			csp[directive] = value + " " + strings.Join(sources, " ")
		case "'none'":
			csp[directive] = strings.Join(sources, " ")
		default:
			for _, source := range sources {
				if !strings.Contains(value, source) {
					value += " " + source
				}
			}
			if !strings.Contains(value, "'self'") {
				value = "'self' " + value
			}
			csp[directive] = value
		}
	}

	values := make([]string, 0, len(csp))
	for directive, value := range csp {
		values = append(values, directive+" "+value)
	}

	// Remove existing CSP header, case insensitive
	for k := range w.Header() {
		if strings.EqualFold(k, "Content-Security-Policy") {
			delete(w.Header(), k)
		}
	}

	// Set new CSP header
	w.Header()["Content-Security-Policy"] = values
}

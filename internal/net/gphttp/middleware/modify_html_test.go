package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestInjectCSS(t *testing.T) {
	opts := OptionsRaw{
		"target": "head",
		"html":   "<style>body { background-color: red; }</style>",
	}
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html; charset=utf-8"},
		},
		respBody: []byte(`
<html>
	<head>
		<title>Test</title>
	</head>
	<body>
		<h1>Test</h1>
	</body>
</html>
		`),
	})
	expect.NoError(t, err)
	expect.Equal(t, removeTabsAndNewlines(result.Data), removeTabsAndNewlines(`
<html>
	<head>
		<title>Test</title>
		<style>body { background-color: red; }</style>
	</head>
	<body>
		<h1>Test</h1>
	</body>
</html>
	`))
	contentLength, _ := strconv.Atoi(result.ResponseHeaders.Get("Content-Length"))
	expect.Equal(t, contentLength, len(result.Data), "Content-Length should be updated")
}

func TestInjectHTML_NonHTMLContent(t *testing.T) {
	opts := OptionsRaw{
		"target": "head",
		"html":   "<style>body { background-color: red; }</style>",
	}
	originalBody := []byte(`{"message": "hello world"}`)
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"application/json"},
		},
		respBody: originalBody,
	})
	expect.NoError(t, err)
	expect.Equal(t, result.Data, originalBody, "Non-HTML content should not be modified")
}

func TestInjectHTML_TargetNotFound(t *testing.T) {
	opts := OptionsRaw{
		"target": ".nonexistent",
		"html":   "<div>This should not appear</div>",
	}
	originalBody := []byte(`
<html>
	<head>
		<title>Test</title>
	</head>
	<body>
		<h1>Test</h1>
	</body>
</html>
	`)
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html"},
		},
		respBody: originalBody,
	})
	expect.NoError(t, err)
	expect.Equal(t, removeTabsAndNewlines(result.Data), removeTabsAndNewlines(string(originalBody)), "Content should remain unchanged when target not found")
}

func TestInjectHTML_MultipleTargets(t *testing.T) {
	opts := OptionsRaw{
		"target": ".container",
		"html":   "<p>Injected content</p>",
	}
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html"},
		},
		respBody: []byte(`
<html>
	<head></head>
	<body>
		<div class="container">First container</div>
		<div class="container">Second container</div>
	</body>
</html>
		`),
	})
	expect.NoError(t, err)
	// Should only inject into the first matching element
	expectedContent := `
<html>
	<head></head>
	<body>
		<div class="container">First container<p>Injected content</p></div>
		<div class="container">Second container</div>
	</body>
</html>
	`
	expect.Equal(t, removeTabsAndNewlines(result.Data), removeTabsAndNewlines(expectedContent))
}

func TestInjectHTML_DifferentSelectors(t *testing.T) {
	testCases := []struct {
		name     string
		selector string
		html     string
		original string
		expected string
	}{
		{
			name:     "ID selector",
			selector: "#main",
			html:     "<span>By ID</span>",
			original: `<div id="main">Content</div>`,
			expected: `<html><head></head><body><div id="main">Content<span>By ID</span></div></body></html>`,
		},
		{
			name:     "Class selector",
			selector: ".highlight",
			html:     "<em>By class</em>",
			original: `<div class="highlight">Content</div>`,
			expected: `<html><head></head><body><div class="highlight">Content<em>By class</em></div></body></html>`,
		},
		{
			name:     "Element selector",
			selector: "body",
			html:     "<footer>Footer content</footer>",
			original: `Content`,
			expected: `<html><head></head><body>Content<footer>Footer content</footer></body></html>`,
		},
		{
			name:     "Attribute selector",
			selector: "[data-test='target']",
			html:     "<b>By attribute</b>",
			original: `<div data-test="target">Content</div>`,
			expected: `<html><head></head><body><div data-test="target">Content<b>By attribute</b></div></body></html>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := OptionsRaw{
				"target": tc.selector,
				"html":   tc.html,
			}

			result, err := newMiddlewareTest(ModifyHTML, &testArgs{
				middlewareOpt: opts,
				respHeaders: http.Header{
					"Content-Type": []string{"text/html"},
				},
				respBody: []byte(`<html><head></head><body>` + tc.original + `</body></html>`),
			})
			expect.NoError(t, err)
			expect.Equal(t, removeTabsAndNewlines(result.Data), removeTabsAndNewlines(tc.expected))
		})
	}
}

func TestInjectHTML_EmptyInjection(t *testing.T) {
	opts := OptionsRaw{
		"target": "head",
		"html":   "",
	}
	originalBody := []byte(`<html><head><title>Test</title></head><body></body></html>`)
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html"},
		},
		respBody: originalBody,
	})
	expect.NoError(t, err)
	expect.Equal(t, removeTabsAndNewlines(result.Data), removeTabsAndNewlines(string(originalBody)), "Empty HTML injection should not change content")
}

func TestInjectHTML_ComplexHTML(t *testing.T) {
	opts := OptionsRaw{
		"target": "body",
		"html":   `<script src="/static/app.js"></script><link rel="stylesheet" href="/static/style.css"/>`,
	}
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html; charset=utf-8"},
		},
		respBody: []byte(`
<!DOCTYPE html>
<html lang="en">
	<head>
		<meta charset="UTF-8"/>
		<title>Complex Page</title>
	</head>
	<body>
		<main>
			<h1>Welcome</h1>
			<p>Some content here.</p>
		</main>
	</body>
</html>
		`),
	})
	expect.NoError(t, err)

	resultStr := removeTabsAndNewlines(result.Data)
	expect.Equal(t, resultStr, removeTabsAndNewlines(`
<!DOCTYPE html>
<html lang="en">
	<head>
		<meta charset="UTF-8"/>
		<title>Complex Page</title>
	</head>
	<body>
		<main>
			<h1>Welcome</h1>
			<p>Some content here.</p>
		</main>
		<script src="/static/app.js"></script><link rel="stylesheet" href="/static/style.css"/>
	</body>
</html>
		`))
	contentLength, _ := strconv.Atoi(result.ResponseHeaders.Get("Content-Length"))
	expect.Equal(t, contentLength, len(result.Data), "Content-Length should be updated correctly")
}

func TestInjectHTML_MalformedHTML(t *testing.T) {
	opts := OptionsRaw{
		"target": "body",
		"html":   "<div>Valid injection</div>",
	}
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html"},
		},
		respBody: []byte(`<html><body><div>Unclosed div<p>Some content</body></html>`),
	})
	expect.NoError(t, err)
	// Should handle malformed HTML gracefully
	expect.True(t, strings.Contains(string(result.Data), "Valid injection"), "Should inject content even with malformed HTML")
}

func TestInjectHTML_ContentTypes(t *testing.T) {
	testCases := []struct {
		name         string
		contentType  string
		shouldModify bool
	}{
		{"HTML with charset", "text/html; charset=utf-8", true},
		{"Plain HTML", "text/html", true},
		{"XHTML", "application/xhtml+xml", true},
		{"JSON", "application/json", false},
		{"Plain text", "text/plain", false},
		{"JavaScript", "application/javascript", false},
		{"CSS", "text/css", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := OptionsRaw{
				"target": "body",
				"html":   "<div>Test injection</div>",
			}
			originalBody := []byte(`<html><body>Original content</body></html>`)
			result, err := newMiddlewareTest(ModifyHTML, &testArgs{
				middlewareOpt: opts,
				respHeaders: http.Header{
					"Content-Type": []string{tc.contentType},
				},
				respBody: originalBody,
			})
			expect.NoError(t, err)

			if tc.shouldModify {
				expect.True(t, strings.Contains(string(result.Data), "Test injection"),
					"Should modify HTML content for content-type: %s", tc.contentType)
			} else {
				expect.Equal(t, string(result.Data), string(originalBody),
					"Should not modify non-HTML content for content-type: %s", tc.contentType)
			}
		})
	}
}

func TestInjectHTML_ReplaceTrue(t *testing.T) {
	opts := OptionsRaw{
		"target":  "body",
		"html":    "<div>Replacement content</div>",
		"replace": true,
	}
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html"},
		},
		respBody: []byte(`
<html>
	<head>
		<title>Test</title>
	</head>
	<body>
		<h1>Original content</h1>
		<p>More original content</p>
	</body>
</html>
		`),
	})
	expect.NoError(t, err)
	expect.Equal(t, removeTabsAndNewlines(result.Data), removeTabsAndNewlines(`
<html>
	<head>
		<title>Test</title>
	</head>
	<body>
		<div>Replacement content</div>
	</body>
</html>
	`))
	contentLength, _ := strconv.Atoi(result.ResponseHeaders.Get("Content-Length"))
	expect.Equal(t, contentLength, len(result.Data), "Content-Length should be updated")
}

func TestInjectHTML_ReplaceVsAppend(t *testing.T) {
	originalBody := []byte(`<html><body><div class="target">Original content</div></body></html>`)

	// Test append behavior (default)
	appendOpts := OptionsRaw{
		"target": ".target",
		"html":   "<span>Added content</span>",
	}
	appendResult, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: appendOpts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html"},
		},
		respBody: originalBody,
	})
	expect.NoError(t, err)
	expect.Equal(t, removeTabsAndNewlines(appendResult.Data), removeTabsAndNewlines(`
<html><head></head><body><div class="target">Original content<span>Added content</span></div></body></html>
	`))

	// Test replace behavior
	replaceOpts := OptionsRaw{
		"target":  ".target",
		"html":    "<span>Replacement content</span>",
		"replace": true,
	}
	replaceResult, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: replaceOpts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html"},
		},
		respBody: originalBody,
	})
	expect.NoError(t, err)
	expect.Equal(t, removeTabsAndNewlines(replaceResult.Data), removeTabsAndNewlines(`
<html><head></head><body><span>Replacement content</span></body></html>
	`))
}

func TestInjectHTML_ReplaceWithDifferentSelectors(t *testing.T) {
	testCases := []struct {
		name     string
		selector string
		original string
		html     string
		expected string
	}{
		{
			name:     "ID selector replace",
			selector: "#main",
			original: `<div id="main">Original content</div>`,
			html:     "<span>Replaced by ID</span>",
			expected: `<html><head></head><body><span>Replaced by ID</span></body></html>`,
		},
		{
			name:     "Class selector replace",
			selector: ".highlight",
			original: `<div class="highlight">Original content</div>`,
			html:     "<em>Replaced by class</em>",
			expected: `<html><head></head><body><em>Replaced by class</em></body></html>`,
		},
		{
			name:     "Element selector replace",
			selector: "body",
			original: `Original content`,
			html:     "<main>Replaced body content</main>",
			expected: `<html><head></head><body><main>Replaced body content</main></body></html>`,
		},
		{
			name:     "Attribute selector replace",
			selector: "[data-test='target']",
			original: `<div data-test="target">Original content</div>`,
			html:     "<b>Replaced by attribute</b>",
			expected: `<html><head></head><body><b>Replaced by attribute</b></body></html>`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := OptionsRaw{
				"target":  tc.selector,
				"html":    tc.html,
				"replace": true,
			}

			result, err := newMiddlewareTest(ModifyHTML, &testArgs{
				middlewareOpt: opts,
				respHeaders: http.Header{
					"Content-Type": []string{"text/html"},
				},
				respBody: []byte(`<html><head></head><body>` + tc.original + `</body></html>`),
			})
			expect.NoError(t, err)
			expect.Equal(t, removeTabsAndNewlines(result.Data), removeTabsAndNewlines(tc.expected))
		})
	}
}

func TestInjectHTML_ReplaceWithEmpty(t *testing.T) {
	opts := OptionsRaw{
		"target":  ".content",
		"html":    "",
		"replace": true,
	}
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html"},
		},
		respBody: []byte(`<html><head></head><body><div class="content">Content to be cleared</div></body></html>`),
	})
	expect.NoError(t, err)
	expect.Equal(t, removeTabsAndNewlines(result.Data), removeTabsAndNewlines(`
<html><head></head><body></body></html>
	`))
}

func TestInjectHTML_ReplaceMultipleTargets(t *testing.T) {
	opts := OptionsRaw{
		"target":  ".container",
		"html":    "<p>Replaced content</p>",
		"replace": true,
	}
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html"},
		},
		respBody: []byte(`
<html>
	<head></head>
	<body>
		<div class="container">First container content</div>
		<div class="container">Second container content</div>
	</body>
</html>
		`),
	})
	expect.NoError(t, err)
	// Should only replace the first matching element
	expectedContent := `
<html>
	<head></head>
	<body>
		<p>Replaced content</p>
		<p>Replaced content</p>
	</body>
</html>
	`
	expect.Equal(t, removeTabsAndNewlines(result.Data), removeTabsAndNewlines(expectedContent))
}

func TestInjectHTML_ReplaceComplexHTML(t *testing.T) {
	opts := OptionsRaw{
		"target":  "main",
		"html":    `<section><h2>New Section</h2><p>This replaces the entire main content.</p></section>`,
		"replace": true,
	}
	result, err := newMiddlewareTest(ModifyHTML, &testArgs{
		middlewareOpt: opts,
		respHeaders: http.Header{
			"Content-Type": []string{"text/html; charset=utf-8"},
		},
		respBody: []byte(`
<!DOCTYPE html>
<html lang="en">
	<head>
		<meta charset="UTF-8"/>
		<title>Complex Page</title>
	</head>
	<body>
		<nav>Navigation</nav>
		<main>
			<h1>Original Title</h1>
			<p>Original content that will be replaced.</p>
			<div>More original content</div>
		</main>
		<footer>Footer</footer>
	</body>
</html>
		`),
	})
	expect.NoError(t, err)

	resultStr := removeTabsAndNewlines(result.Data)
	expect.Equal(t, resultStr, removeTabsAndNewlines(`
<!DOCTYPE html>
<html lang="en">
	<head>
		<meta charset="UTF-8"/>
		<title>Complex Page</title>
	</head>
	<body>
		<nav>Navigation</nav>
		<section><h2>New Section</h2><p>This replaces the entire main content.</p></section>
		<footer>Footer</footer>
	</body>
</html>
		`))
	contentLength, _ := strconv.Atoi(result.ResponseHeaders.Get("Content-Length"))
	expect.Equal(t, contentLength, len(result.Data), "Content-Length should be updated correctly")
}

func removeTabsAndNewlines[T string | []byte](s T) string {
	replacer := strings.NewReplacer(
		"\n", "",
		"\r", "",
		"\t", "",
	)
	return replacer.Replace(string(s))
}

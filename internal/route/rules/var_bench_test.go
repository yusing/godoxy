package rules

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	httputils "github.com/yusing/goutils/http"
)

func BenchmarkExpandVars(b *testing.B) {
	testResponseModifier := httputils.NewResponseModifier(httptest.NewRecorder())
	testResponseModifier.WriteHeader(http.StatusOK)
	testResponseModifier.Write([]byte("Hello, world!"))
	testRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	testRequest.Header.Set("User-Agent", "test-agent/1.0")
	testRequest.Header.Set("X-Custom", "value1,value2")
	testRequest.ContentLength = 12345
	testRequest.RemoteAddr = "192.168.1.100:54321"
	testRequest.Form = url.Values{"param1": {"value1"}, "param2": {"value2"}}
	testRequest.PostForm = url.Values{"param3": {"value3"}, "param4": {"value4"}}

	for b.Loop() {
		_, err := ExpandVars(testResponseModifier, testRequest, "$req_method $req_path $req_query $req_url $req_uri $req_host $req_port $req_addr $req_content_type $req_content_length $remote_host $remote_port $remote_addr $status_code $resp_content_type $resp_content_length $header(User-Agent) $header(X-Custom, 0) $header(X-Custom, 1) $arg(param1) $arg(param2) $arg(param3) $arg(param4) $form(param1) $form(param2) $form(param3) $form(param4) $postform(param1) $postform(param2) $postform(param3) $postform(param4)", io.Discard)
		if err != nil {
			b.Fatal(err)
		}
	}
}

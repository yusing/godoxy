package query

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	v1 "github.com/yusing/go-proxy/internal/api/v1"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware"
)

func ReloadServer() gperr.Error {
	resp, err := gphttp.Post(common.APIHTTPURL+"/v1/reload", "", nil)
	if err != nil {
		return gperr.Wrap(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		failure := gperr.Errorf("server reload status %v", resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return failure.With(err)
		}
		reloadErr := string(body)
		return failure.Withf(reloadErr)
	}
	return nil
}

func List[T any](what string) (_ T, outErr gperr.Error) {
	resp, err := gphttp.Get(fmt.Sprintf("%s/v1/list/%s", common.APIHTTPURL, what))
	if err != nil {
		outErr = gperr.Wrap(err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		outErr = gperr.Errorf("list %s: failed, status %v", what, resp.StatusCode)
		return
	}
	var res T
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		outErr = gperr.Wrap(err)
		return
	}
	return res, nil
}

func ListRoutes() (map[string]map[string]any, gperr.Error) {
	return List[map[string]map[string]any](v1.ListRoutes)
}

func ListMiddlewareTraces() (middleware.Traces, gperr.Error) {
	return List[middleware.Traces](v1.ListMiddlewareTraces)
}

func DebugListTasks() (map[string]any, gperr.Error) {
	return List[map[string]any](v1.ListTasks)
}

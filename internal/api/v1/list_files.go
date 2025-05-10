package v1

import (
	"net/http"
	"strings"

	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/utils"
)

func ListFilesHandler(cfg config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	files, err := utils.ListFiles(common.ConfigBasePath, 0, true)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}
	resp := map[FileType][]string{
		FileTypeConfig:     make([]string, 0),
		FileTypeProvider:   make([]string, 0),
		FileTypeMiddleware: make([]string, 0),
	}

	for _, file := range files {
		t := fileType(file)
		file = strings.TrimPrefix(file, common.ConfigBasePath+"/")
		resp[t] = append(resp[t], file)
	}

	mids, err := utils.ListFiles(common.MiddlewareComposeBasePath, 0, true)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}
	for _, mid := range mids {
		mid = strings.TrimPrefix(mid, common.MiddlewareComposeBasePath+"/")
		resp[FileTypeMiddleware] = append(resp[FileTypeMiddleware], mid)
	}
	gphttp.RespondJSON(w, r, resp)
}

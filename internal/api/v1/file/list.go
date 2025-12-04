package fileapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/common"
	apitypes "github.com/yusing/goutils/apitypes"
	"github.com/yusing/goutils/fs"
)

type ListFilesResponse struct {
	Config     []string `json:"config"`
	Provider   []string `json:"provider"`
	Middleware []string `json:"middleware"`
} // @name ListFilesResponse

// @x-id				"list"
// @BasePath		/api/v1
// @Summary		List files
// @Description	List files
// @Tags			file
// @Accept			json
// @Produce		json
// @Success		200	{object}	ListFilesResponse
// @Failure		403	{object}	apitypes.ErrorResponse
// @Failure		500	{object}	apitypes.ErrorResponse
// @Router			/file/list [get]
func List(c *gin.Context) {
	resp := map[FileType][]string{
		FileTypeConfig:     make([]string, 0),
		FileTypeProvider:   make([]string, 0),
		FileTypeMiddleware: make([]string, 0),
	}

	// config/
	files, err := fs.ListFiles(common.ConfigBasePath, 0, true)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to list files"))
		return
	}

	for _, file := range files {
		t := GetFileType(file)
		file = strings.TrimPrefix(file, common.ConfigBasePath+"/")
		resp[t] = append(resp[t], file)
	}

	// config/middlewares/
	mids, err := fs.ListFiles(common.MiddlewareComposeBasePath, 0, true)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to list files"))
		return
	}
	for _, mid := range mids {
		mid = strings.TrimPrefix(mid, common.MiddlewareComposeBasePath+"/")
		resp[FileTypeMiddleware] = append(resp[FileTypeMiddleware], mid)
	}

	c.JSON(http.StatusOK, resp)
}

package fileapi

import (
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/yusing/godoxy/internal/common"
	apitypes "github.com/yusing/goutils/apitypes"
)

type FileType string // @name FileType

const (
	FileTypeConfig     FileType = "config"     // @name FileTypeConfig
	FileTypeProvider   FileType = "provider"   // @name FileTypeProvider
	FileTypeMiddleware FileType = "middleware" // @name FileTypeMiddleware
)

type GetFileContentRequest struct {
	FileType FileType `form:"type" binding:"required,oneof=config provider middleware"`
	Filename string   `form:"filename" binding:"required" format:"filename"`
} //	@name	GetFileContentRequest

// @x-id				"get"
// @BasePath		/api/v1
// @Summary		Get file content
// @Description	Get file content
// @Tags			file
// @Accept			json
// @Produce		application/godoxy+yaml
// @Param			query	query		GetFileContentRequest	true	"Request"
// @Success		200			{string}	application/godoxy+yaml	"File content"
// @Failure		400			{object}	apitypes.ErrorResponse
// @Failure		403			{object}	apitypes.ErrorResponse
// @Failure		500			{object}	apitypes.ErrorResponse
// @Router			/file/content [get]
func Get(c *gin.Context) {
	var request GetFileContentRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	f, err := request.FileType.OpenFile(request.Filename, os.O_RDONLY, 0)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to open root"))
		return
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to read file"))
		return
	}

	// RFC 9512: https://www.rfc-editor.org/rfc/rfc9512.html
	// xxx/yyy+yaml
	c.Data(http.StatusOK, "application/godoxy+yaml", content)
}

func GetFileType(file string) FileType {
	switch {
	case strings.HasPrefix(path.Base(file), "config."):
		return FileTypeConfig
	case strings.HasPrefix(file, common.MiddlewareComposeBasePath):
		return FileTypeMiddleware
	}
	return FileTypeProvider
}

func (t FileType) RootPath() string {
	if t == FileTypeMiddleware {
		return common.MiddlewareComposeBasePath
	}
	return common.ConfigBasePath
}

func (t FileType) OpenFile(filename string, flag int, perm os.FileMode) (*os.File, error) {
	root, err := os.OpenRoot(t.RootPath())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.OpenFile(filename, flag, perm)
}

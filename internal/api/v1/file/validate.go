package fileapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware"
	"github.com/yusing/go-proxy/internal/route/provider"
)

type ValidateFileRequest struct {
	FileType FileType `form:"type" validate:"required,oneof=config provider middleware"`
} //	@name	ValidateFileRequest

// @x-id				"validate"
// @BasePath		/api/v1
// @Summary		Validate file
// @Description	Validate file
// @Tags			file
// @Accept			json
// @Produce		json
// @Param			type	query		FileType	true	"Type"
// @Param			file	body		string		true	"File content"
// @Success		200		{object}	apitypes.SuccessResponse "File validated"
// @Failure		400		{object}	apitypes.ErrorResponse "Bad request"
// @Failure		403		{object}	apitypes.ErrorResponse "Forbidden"
// @Failure		417		{object}	apitypes.ErrorResponse "Validation failed"
// @Failure		500		{object}	apitypes.ErrorResponse "Internal server error"
// @Router			/file/validate [post]
func Validate(c *gin.Context) {
	var request ValidateFileRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	content, err := c.GetRawData()
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to read file"))
		return
	}
	c.Request.Body.Close()

	if valErr := validateFile(request.FileType, content); valErr != nil {
		c.JSON(http.StatusExpectationFailed, apitypes.Error("invalid file", valErr))
		return
	}
	c.JSON(http.StatusOK, apitypes.Success("file validated"))
}

func validateFile(fileType FileType, content []byte) gperr.Error {
	switch fileType {
	case FileTypeConfig:
		return config.Validate(content)
	case FileTypeMiddleware:
		errs := gperr.NewBuilder("middleware errors")
		middleware.BuildMiddlewaresFromYAML("", content, errs)
		return errs.Error()
	}
	return provider.Validate(content)
}

package fileapi

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	apitypes "github.com/yusing/go-proxy/internal/api/types"
)

type SetFileContentRequest GetFileContentRequest

// @x-id				"set"
// @BasePath		/api/v1
// @Summary		Set file content
// @Description	Set file content
// @Tags			file
// @Accept			json
// @Produce		json
// @Param			type		query		FileType	true	"Type"
// @Param			filename	query		string		true	"Filename"
// @Param			file		body		string		true	"File"
// @Success		200			{object}	apitypes.SuccessResponse
// @Failure		400			{object}	apitypes.ErrorResponse
// @Failure		403			{object}	apitypes.ErrorResponse
// @Failure		500			{object}	apitypes.ErrorResponse
// @Router			/file/content [put]
func Set(c *gin.Context) {
	var request SetFileContentRequest
	if err := c.ShouldBindQuery(&request); err != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid request", err))
		return
	}

	content, err := c.GetRawData()
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to read file"))
		return
	}

	if valErr := validateFile(request.FileType, content); valErr != nil {
		c.JSON(http.StatusBadRequest, apitypes.Error("invalid file", valErr))
		return
	}

	err = os.WriteFile(request.FileType.GetPath(request.Filename), content, 0o644)
	if err != nil {
		c.Error(apitypes.InternalServerError(err, "failed to write file"))
		return
	}
	c.JSON(http.StatusOK, apitypes.Success("file set"))
}

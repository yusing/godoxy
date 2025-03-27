package v1

import (
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware"
	"github.com/yusing/go-proxy/internal/route/provider"
)

type FileType string

const (
	FileTypeConfig     FileType = "config"
	FileTypeProvider   FileType = "provider"
	FileTypeMiddleware FileType = "middleware"
)

func fileType(file string) FileType {
	switch {
	case strings.HasPrefix(path.Base(file), "config."):
		return FileTypeConfig
	case strings.HasPrefix(file, common.MiddlewareComposeBasePath):
		return FileTypeMiddleware
	}
	return FileTypeProvider
}

func (t FileType) IsValid() bool {
	switch t {
	case FileTypeConfig, FileTypeProvider, FileTypeMiddleware:
		return true
	}
	return false
}

func (t FileType) GetPath(filename string) string {
	if t == FileTypeMiddleware {
		return path.Join(common.MiddlewareComposeBasePath, filename)
	}
	return path.Join(common.ConfigBasePath, filename)
}

func getArgs(r *http.Request) (fileType FileType, filename string, err error) {
	fileType = FileType(r.PathValue("type"))
	if !fileType.IsValid() {
		err = gphttp.ErrInvalidKey("type")
		return
	}
	filename = r.PathValue("filename")
	if filename == "" {
		err = gphttp.ErrMissingKey("filename")
	}
	return
}

func GetFileContent(w http.ResponseWriter, r *http.Request) {
	fileType, filename, err := getArgs(r)
	if err != nil {
		gphttp.BadRequest(w, err.Error())
		return
	}
	content, err := os.ReadFile(fileType.GetPath(filename))
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}
	gphttp.WriteBody(w, content)
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

func ValidateFile(w http.ResponseWriter, r *http.Request) {
	fileType := FileType(r.PathValue("type"))
	if !fileType.IsValid() {
		gphttp.BadRequest(w, "invalid file type")
		return
	}
	content, err := io.ReadAll(r.Body)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}
	r.Body.Close()
	if valErr := validateFile(fileType, content); valErr != nil {
		gphttp.JSONError(w, valErr, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func SetFileContent(w http.ResponseWriter, r *http.Request) {
	fileType, filename, err := getArgs(r)
	if err != nil {
		gphttp.BadRequest(w, err.Error())
		return
	}
	content, err := io.ReadAll(r.Body)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}

	if valErr := validateFile(fileType, content); valErr != nil {
		gphttp.JSONError(w, valErr, http.StatusBadRequest)
		return
	}

	err = os.WriteFile(fileType.GetPath(filename), content, 0o644)
	if err != nil {
		gphttp.ServerError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

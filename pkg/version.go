package pkg

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
)

func GetVersion() Version {
	return currentVersion
}

func GetLastVersion() Version {
	return lastVersion
}

func GetVersionHTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(GetVersion().String()))
	}
}

func init() {
	currentVersion = ParseVersion(version)

	// ignore errors
	versionFile := filepath.Join(common.DataDir, "version")
	var lastVersionStr string
	f, err := os.OpenFile(versionFile, os.O_RDWR|os.O_CREATE, 0o644)
	if err == nil {
		_, err = fmt.Fscanf(f, "%s", &lastVersionStr)
		lastVersion = ParseVersion(lastVersionStr)
	}
	if err != nil && !os.IsNotExist(err) {
		logging.Warn().Err(err).Msg("failed to read version file")
		return
	}
	if err := f.Truncate(0); err != nil {
		logging.Warn().Err(err).Msg("failed to truncate version file")
		return
	}
	_, err = f.WriteString(version)
	if err != nil {
		logging.Warn().Err(err).Msg("failed to save version file")
		return
	}
}

type Version struct{ Generation, Major, Minor int }

func Ver(major, minor, patch int) Version {
	return Version{major, minor, patch}
}

func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Generation, v.Major, v.Minor)
}

func (v Version) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

func (v Version) IsNewerMajorThan(other Version) bool {
	if v.Generation != other.Generation {
		return v.Generation > other.Generation
	}
	return v.Major > other.Major
}

func (v Version) IsEqual(other Version) bool {
	return v.Generation == other.Generation && v.Major == other.Major && v.Minor == other.Minor
}

var (
	version        = "unset"
	currentVersion Version
	lastVersion    Version
)

var versionRegex = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(\-\w+)?$`)

func ParseVersion(v string) (ver Version) {
	if v == "" {
		return
	}

	if !versionRegex.MatchString(v) { // likely feature branch (e.g. feat/some-feature)
		return
	}

	v = strings.Split(v, "-")[0]
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return
	}
	gen, err := strconv.Atoi(parts[0])
	if err != nil {
		return
	}
	major, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	minor, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	return Ver(gen, major, minor)
}

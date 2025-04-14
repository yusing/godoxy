package pkg

import (
	"fmt"
	"os"
	"path/filepath"
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

func init() {
	currentVersion = parseVersion(version)

	// ignore errors
	versionFile := filepath.Join(common.DataDir, "version")
	var lastVersionStr string
	f, err := os.OpenFile(versionFile, os.O_RDWR|os.O_CREATE, 0o644)
	if err == nil {
		_, err = fmt.Fscanf(f, "%s", &lastVersionStr)
		lastVersion = parseVersion(lastVersionStr)
	}
	if err != nil && !os.IsNotExist(err) {
		logging.Warn().Err(err).Msg("failed to read version file")
	}
	_, err = f.WriteString(version)
	if err != nil {
		logging.Warn().Err(err).Msg("failed to save version file")
	}
}

type Version struct{ Major, Minor, Patch int }

func Ver(major, minor, patch int) Version {
	return Version{major, minor, patch}
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v Version) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

func (v Version) IsNewerThan(other Version) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor > other.Minor
	}
	return v.Patch > other.Patch
}

func (v Version) IsOlderThan(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

func (v Version) IsEqual(other Version) bool {
	return v.Major == other.Major && v.Minor == other.Minor && v.Patch == other.Patch
}

var (
	version        = "unset"
	currentVersion Version
	lastVersion    Version
)

func parseVersion(v string) (ver Version) {
	if v == "" {
		return
	}

	v = strings.Split(v, "-")[0]
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	return Ver(major, minor, patch)
}

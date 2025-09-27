package pkg

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

func GetVersion() Version {
	return currentVersion
}

func GetLastVersion() Version {
	return lastVersion
}

func init() {
	currentVersion = ParseVersion(version)

	// ignore errors
	// versionFile := filepath.Join(common.DataDir, "version")
	// var lastVersionStr string
	// f, err := os.OpenFile(versionFile, os.O_RDWR|os.O_CREATE, 0o644)
	// if err == nil {
	// 	_, err = fmt.Fscanf(f, "%s", &lastVersionStr)
	// 	lastVersion = ParseVersion(lastVersionStr)
	// }
	// if err != nil && !os.IsNotExist(err) {
	// 	log.Warn().Err(err).Msg("failed to read version file")
	// 	return
	// }
	// if err := f.Truncate(0); err != nil {
	// 	log.Warn().Err(err).Msg("failed to truncate version file")
	// 	return
	// }
	// _, err = f.WriteString(version)
	// if err != nil {
	// 	log.Warn().Err(err).Msg("failed to save version file")
	// 	return
	// }
}

type Version struct{ Generation, Major, Minor int }

func Ver(gen, major, minor int) Version {
	return Version{gen, major, minor}
}

func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Generation, v.Major, v.Minor)
}

func (v Version) MarshalText() ([]byte, error) {
	return []byte(v.String()), nil
}

func (v Version) IsNewerThan(other Version) bool {
	if v.Generation != other.Generation {
		return v.Generation > other.Generation
	}
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	return v.Minor > other.Minor
}

func (v Version) IsNewerThanMajor(other Version) bool {
	if v.Generation != other.Generation {
		return v.Generation > other.Generation
	}
	return v.Major > other.Major
}

func (v Version) IsOlderThan(other Version) bool {
	return !v.IsNewerThan(other)
}

func (v Version) IsOlderThanMajor(other Version) bool {
	if v.Generation != other.Generation {
		return v.Generation < other.Generation
	}
	return v.Major < other.Major
}

func (v Version) IsOlderMajorThan(other Version) bool {
	return !v.IsNewerThanMajor(other)
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
		return ver
	}

	if !versionRegex.MatchString(v) { // likely feature branch (e.g. feat/some-feature)
		return ver
	}

	v = strings.Split(v, "-")[0]
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return ver
	}
	gen, err := strconv.Atoi(parts[0])
	if err != nil {
		return ver
	}
	major, err := strconv.Atoi(parts[1])
	if err != nil {
		return ver
	}
	minor, err := strconv.Atoi(parts[2])
	if err != nil {
		return ver
	}
	return Ver(gen, major, minor)
}

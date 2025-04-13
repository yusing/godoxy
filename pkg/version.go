package pkg

import (
	"fmt"
	"strconv"
	"strings"
)

func GetVersion() Version {
	return Version{Major: major, Minor: minor, Patch: patch}
}

func init() {
	major, minor, patch = parseVersion(version)
}

type Version struct{ Major, Minor, Patch int }

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
	version             = "unset"
	major, minor, patch int
)

func parseVersion(v string) (major, minor, patch int) {
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
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return
	}
	return
}

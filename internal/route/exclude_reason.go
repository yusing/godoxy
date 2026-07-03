package route

import (
	"strconv"
)

type ExcludedReason uint8

const (
	ExcludedReasonNone ExcludedReason = iota
	ExcludedReasonError
	ExcludedReasonManual
	ExcludedReasonNoPortContainer
	ExcludedReasonBlacklisted
	ExcludedReasonBuildx
	ExcludedReasonYAMLAnchor
	ExcludedReasonOld
)

func (re ExcludedReason) String() string {
	switch re {
	case ExcludedReasonNone:
		return ""
	case ExcludedReasonError:
		return "Error"
	case ExcludedReasonManual:
		return "Manual exclusion"
	case ExcludedReasonNoPortContainer:
		return "No port exposed in container"
	case ExcludedReasonBlacklisted:
		return "Blacklisted (backend service or database)"
	case ExcludedReasonBuildx:
		return "Buildx"
	case ExcludedReasonYAMLAnchor:
		return "YAML anchor or reference"
	case ExcludedReasonOld:
		return "Container renaming intermediate state"
	default:
		return "Unknown"
	}
}

func (re ExcludedReason) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, re.String()), nil
}

// no need to unmarshal json because we don't store this

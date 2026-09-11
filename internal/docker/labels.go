package docker

import "strconv"

type ExcludeFlag uint8

const (
	ExcludeProxy ExcludeFlag = 1 << iota
	ExcludeHealthCheck
	// ExcludeAll must remain last so it includes every defined exclusion flag.
	ExcludeAll ExcludeFlag = 1<<iota - 1
)

func (flags ExcludeFlag) String() string {
	switch flags {
	case 0:
		return "none"
	case ExcludeProxy:
		return "proxy"
	case ExcludeHealthCheck:
		return "healthcheck"
	case ExcludeAll:
		return "all"
	default:
		return "unknown"
	}
}

func (flags ExcludeFlag) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, flags.String()), nil
}

const (
	WildcardAlias = "*"

	NSProxy = "proxy"

	LabelAliases       = NSProxy + ".aliases"
	LabelExclude       = NSProxy + ".exclude"
	LabelIdleTimeout   = NSProxy + ".idle_timeout"
	LabelWakeTimeout   = NSProxy + ".wake_timeout"
	LabelStopMethod    = NSProxy + ".stop_method"
	LabelStopTimeout   = NSProxy + ".stop_timeout"
	LabelStopSignal    = NSProxy + ".stop_signal"
	LabelStartEndpoint = NSProxy + ".start_endpoint"
	LabelDependsOn     = NSProxy + ".depends_on"
	LabelNoLoadingPage = NSProxy + ".no_loading_page" // No loading page when using idlewatcher
	LabelNetwork       = NSProxy + ".network"

	// Sleep/wake notifications. `to` is comma separated.
	LabelIdleNotify   = NSProxy + ".idle_notify"
	LabelIdleNotifyTo = NSProxy + ".idle_notify_to"
)

// key: label, value: dot separated key path in IdlewatcherConfig
var idlewatcherLabels = map[string]string{
	LabelIdleTimeout:   "idle_timeout",
	LabelWakeTimeout:   "wake_timeout",
	LabelStopMethod:    "stop_method",
	LabelStopTimeout:   "stop_timeout",
	LabelStopSignal:    "stop_signal",
	LabelStartEndpoint: "start_endpoint",
	LabelDependsOn:     "depends_on",
	LabelNoLoadingPage: "no_loading_page",
	LabelIdleNotify:    "notify.enabled",
	LabelIdleNotifyTo:  "notify.to",
}

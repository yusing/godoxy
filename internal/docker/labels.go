package docker

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
)

// key: label, value: key in IdlewatcherConfig
var idlewatcherLabels = map[string]string{
	LabelIdleTimeout:   "idle_timeout",
	LabelWakeTimeout:   "wake_timeout",
	LabelStopMethod:    "stop_method",
	LabelStopTimeout:   "stop_timeout",
	LabelStopSignal:    "stop_signal",
	LabelStartEndpoint: "start_endpoint",
	LabelDependsOn:     "depends_on",
	LabelNoLoadingPage: "no_loading_page",
}

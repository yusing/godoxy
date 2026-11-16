package gpu

var factories = map[string]func() (Collector, error){
	"intel": func() (Collector, error) { return newIntelCollector(sysfsPMURoot, openPerfCounter) },
}

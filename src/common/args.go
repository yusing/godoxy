package common

import (
	"flag"

	"github.com/sirupsen/logrus"
	E "github.com/yusing/go-proxy/error"
)

type Args struct {
	Command string
}

const (
	CommandStart            = ""
	CommandValidate         = "validate"
	CommandListConfigs      = "ls-config"
	CommandListRoutes       = "ls-routes"
	CommandReload           = "reload"
	CommandDebugListEntries = "debug-ls-entries"
)

var ValidCommands = []string{
	CommandStart,
	CommandValidate,
	CommandListConfigs,
	CommandListRoutes,
	CommandReload,
	CommandDebugListEntries,
}

func GetArgs() Args {
	var args Args
	flag.Parse()
	args.Command = flag.Arg(0)
	if err := validateArg(args.Command); err.HasError() {
		logrus.Fatal(err)
	}
	return args
}

func validateArg(arg string) E.NestedError {
	for _, v := range ValidCommands {
		if arg == v {
			return nil
		}
	}
	return E.Invalid("argument", arg)
}

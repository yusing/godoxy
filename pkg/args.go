package pkg

import (
	"flag"
	"log"
)

type (
	Args struct {
		Command string
		Args    []string
	}
	CommandValidator interface {
		IsCommandValid(cmd string) bool
	}
)

func GetArgs(validator CommandValidator) Args {
	var args Args
	flag.Parse()
	args.Command = flag.Arg(0)
	if !validator.IsCommandValid(args.Command) {
		log.Fatalf("invalid command: %s", args.Command)
	}
	if len(flag.Args()) > 1 {
		args.Args = flag.Args()[1:]
	}
	return args
}

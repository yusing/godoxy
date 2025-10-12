package rules

import "net/http"

type (
	CommandHandler interface {
		// CommandHandler can read and modify the values
		// then handle the request
		// finally proceed to next command (or return) base on situation
		Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool)
	}
	// NonTerminatingCommand will run then proceed to next command or reverse proxy.
	NonTerminatingCommand http.HandlerFunc
	// TerminatingCommand will run then return immediately.
	TerminatingCommand http.HandlerFunc
	// DynamicCommand will return base on the request
	// and can read or modify the values.
	DynamicCommand func(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool)
	// BypassCommand will skip all the following commands
	// and directly return to reverse proxy.
	BypassCommand struct{}
	// Commands is a slice of CommandHandler.
	Commands []CommandHandler
)

func (c NonTerminatingCommand) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	c(w, r)
	return true
}

func (c TerminatingCommand) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	c(w, r)
	return false
}

func (c DynamicCommand) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	return c(cached, w, r)
}

func (c BypassCommand) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	return true
}

func (c Commands) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	for _, cmd := range c {
		if !cmd.Handle(cached, w, r) {
			return false
		}
	}
	return true
}

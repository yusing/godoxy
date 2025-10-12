package rules

import "net/http"

type (
	CommandHandler interface {
		// CommandHandler can read and modify the values
		// then handle the request
		// finally proceed to next command (or return) base on situation
		Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool)
		IsResponseHandler() bool
	}
	// NonTerminatingCommand will run then proceed to next command or reverse proxy.
	NonTerminatingCommand http.HandlerFunc
	// TerminatingCommand will run then return immediately.
	TerminatingCommand http.HandlerFunc
	// DynamicCommand will return base on the request
	// and can read or modify the values.
	DynamicCommand    func(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool)
	OnResponseCommand http.HandlerFunc
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

func (c NonTerminatingCommand) IsResponseHandler() bool {
	return false
}

func (c TerminatingCommand) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	c(w, r)
	return false
}

func (c TerminatingCommand) IsResponseHandler() bool {
	return false
}

func (c DynamicCommand) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	return c(cached, w, r)
}

func (c DynamicCommand) IsResponseHandler() bool {
	return false
}

func (c OnResponseCommand) Handle(_ Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	c(w, r)
	return true
}

func (c OnResponseCommand) IsResponseHandler() bool {
	return true
}

func (c BypassCommand) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	return true
}

func (c BypassCommand) IsResponseHandler() bool {
	return false
}

func (c Commands) Handle(cached Cache, w http.ResponseWriter, r *http.Request) (proceed bool) {
	for _, cmd := range c {
		if !cmd.Handle(cached, w, r) {
			return false
		}
	}
	return true
}

func (c Commands) IsResponseHandler() bool {
	for _, cmd := range c {
		if cmd.IsResponseHandler() {
			return true
		}
	}
	return false
}

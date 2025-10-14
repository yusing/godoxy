package rules

import "net/http"

type (
	handlerFunc func(w http.ResponseWriter, r *http.Request) error

	CommandHandler interface {
		// CommandHandler can read and modify the values
		// then handle the request
		// finally proceed to next command (or return) base on situation
		Handle(w http.ResponseWriter, r *http.Request) error
		IsResponseHandler() bool
	}
	// NonTerminatingCommand will run then proceed to next command or reverse proxy.
	NonTerminatingCommand handlerFunc
	// TerminatingCommand will run then return immediately.
	TerminatingCommand handlerFunc
	// OnResponseCommand will run then return based on the response.
	OnResponseCommand handlerFunc
	// BypassCommand will skip all the following commands
	// and directly return to reverse proxy.
	BypassCommand struct{}
	// Commands is a slice of CommandHandler.
	Commands []CommandHandler
)

func (c NonTerminatingCommand) Handle(w http.ResponseWriter, r *http.Request) error {
	return c(w, r)
}

func (c NonTerminatingCommand) IsResponseHandler() bool {
	return false
}

func (c TerminatingCommand) Handle(w http.ResponseWriter, r *http.Request) error {
	if err := c(w, r); err != nil {
		return err
	}
	return errTerminated
}

func (c TerminatingCommand) IsResponseHandler() bool {
	return false
}

func (c OnResponseCommand) Handle(w http.ResponseWriter, r *http.Request) error {
	return c(w, r)
}

func (c OnResponseCommand) IsResponseHandler() bool {
	return true
}

func (c BypassCommand) Handle(w http.ResponseWriter, r *http.Request) error {
	return errTerminated
}

func (c BypassCommand) IsResponseHandler() bool {
	return false
}

func (c Commands) Handle(w http.ResponseWriter, r *http.Request) error {
	for _, cmd := range c {
		if err := cmd.Handle(w, r); err != nil {
			return err
		}
	}
	return nil
}

func (c Commands) IsResponseHandler() bool {
	for _, cmd := range c {
		if cmd.IsResponseHandler() {
			return true
		}
	}
	return false
}

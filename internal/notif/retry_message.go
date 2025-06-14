package notif

import (
	"time"

	"github.com/rs/zerolog"
)

type RetryMessage struct {
	Message   *LogMessage
	Trials    int
	Provider  Provider
	NextRetry time.Time
}

var maxRetries = map[zerolog.Level]int{
	zerolog.DebugLevel: 1,
	zerolog.InfoLevel:  1,
	zerolog.WarnLevel:  3,
	zerolog.ErrorLevel: 5,
	zerolog.FatalLevel: 10,
	zerolog.PanicLevel: 10,
}

func (msg *RetryMessage) MarshalZerologObject(e *zerolog.Event) {
	e.Str("provider", msg.Provider.GetName()).
		Int("trial", msg.Trials+1).
		Str("title", msg.Message.Title)
	if !msg.NextRetry.IsZero() {
		e.Int("max_retries", maxRetries[msg.Message.Level]).
			Time("next_retry", msg.NextRetry)
	}
}

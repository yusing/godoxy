package notif

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/task"
	F "github.com/yusing/go-proxy/internal/utils/functional"
)

type (
	Dispatcher struct {
		task        *task.Task
		providers   F.Set[Provider]
		logCh       chan *LogMessage
		retryMsg    F.Set[*RetryMessage]
		retryTicker *time.Ticker
	}
	LogMessage struct {
		Level zerolog.Level
		Title string
		Body  LogBody
		Color Color
	}
)

var dispatcher *Dispatcher

const (
	retryInterval     = time.Second
	maxBackoffDelay   = 5 * time.Minute
	backoffMultiplier = 2.0
)

func StartNotifDispatcher(parent task.Parent) *Dispatcher {
	dispatcher = &Dispatcher{
		task:        parent.Subtask("notification", true),
		providers:   F.NewSet[Provider](),
		logCh:       make(chan *LogMessage, 100),
		retryMsg:    F.NewSet[*RetryMessage](),
		retryTicker: time.NewTicker(retryInterval),
	}
	go dispatcher.start()
	return dispatcher
}

func Notify(msg *LogMessage) {
	if dispatcher == nil {
		return
	}
	select {
	case <-dispatcher.task.Context().Done():
		return
	default:
		dispatcher.logCh <- msg
	}
}

func (disp *Dispatcher) RegisterProvider(cfg *NotificationConfig) {
	disp.providers.Add(cfg.Provider)
}

func (disp *Dispatcher) start() {
	defer func() {
		disp.providers.Clear()
		close(disp.logCh)
		disp.task.Finish(nil)
		dispatcher = nil
	}()

	for {
		select {
		case <-disp.task.Context().Done():
			return
		case msg, ok := <-disp.logCh:
			if !ok {
				return
			}
			go disp.dispatch(msg)
		case <-disp.retryTicker.C:
			disp.processRetries()
		}
	}
}

func (disp *Dispatcher) dispatch(msg *LogMessage) {
	task := disp.task.Subtask("dispatcher", true)
	defer task.Finish("notif dispatched")

	l := log.With().
		Str("level", msg.Level.String()).
		Str("title", msg.Title).Logger()

	disp.providers.RangeAllParallel(func(p Provider) {
		if err := msg.notify(task.Context(), p); err != nil {
			msg := &RetryMessage{
				Message:   msg,
				Trials:    0,
				Provider:  p,
				NextRetry: time.Now().Add(calculateBackoffDelay(0)),
			}
			disp.retryMsg.Add(msg)
			l.Debug().Err(err).EmbedObject(msg).Msg("notification failed, scheduling retry")
		} else {
			l.Debug().Str("provider", p.GetName()).Msg("notification sent successfully")
		}
	})
}

func (disp *Dispatcher) processRetries() {
	if disp.retryMsg.Size() == 0 {
		return
	}

	now := time.Now()

	readyMessages := make([]*RetryMessage, 0)
	for msg := range disp.retryMsg.Range {
		if now.After(msg.NextRetry) {
			readyMessages = append(readyMessages, msg)
			disp.retryMsg.Remove(msg)
		}
	}

	disp.retry(readyMessages)
}

func (disp *Dispatcher) retry(messages []*RetryMessage) {
	if len(messages) == 0 {
		return
	}

	task := disp.task.Subtask("retry", true)
	defer task.Finish("notif retried")

	successCount := 0
	failureCount := 0

	for _, msg := range messages {
		maxTrials := maxRetries[msg.Message.Level]
		log.Debug().EmbedObject(msg).Msg("attempting notification retry")

		err := msg.Message.notify(task.Context(), msg.Provider)
		if err == nil {
			msg.NextRetry = time.Time{}
			successCount++
			log.Debug().EmbedObject(msg).Msg("notification retry succeeded")
			continue
		}

		msg.Trials++
		failureCount++

		if msg.Trials >= maxTrials {
			log.Warn().Err(err).EmbedObject(msg).Msg("notification permanently failed after max retries")
			continue
		}

		// Schedule next retry with exponential backoff
		msg.NextRetry = time.Now().Add(calculateBackoffDelay(msg.Trials))
		disp.retryMsg.Add(msg)

		log.Debug().EmbedObject(msg).Msg("notification retry failed, scheduled for later")
	}

	log.Info().
		Int("total", len(messages)).
		Int("successes", successCount).
		Int("failures", failureCount).
		Msg("notification retry batch completed")
}

// calculateBackoffDelay implements exponential backoff with jitter.
func calculateBackoffDelay(trials int) time.Duration {
	if trials == 0 {
		return retryInterval
	}

	// Exponential backoff: retryInterval * (backoffMultiplier ^ trials)
	delay := min(float64(retryInterval)*math.Pow(backoffMultiplier, float64(trials)), float64(maxBackoffDelay))

	// Add 20% jitter to prevent thundering herd
	//nolint:gosec
	jitter := delay * 0.2 * (rand.Float64() - 0.5) // -10% to +10%
	return time.Duration(delay + jitter)
}

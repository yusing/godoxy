package notif

import (
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/task"
	F "github.com/yusing/go-proxy/internal/utils/functional"
)

type (
	Dispatcher struct {
		task        *task.Task
		providers   F.Set[Provider]
		logCh       chan *LogMessage
		retryCh     chan *RetryMessage
		retryTicker *time.Ticker
	}
	LogMessage struct {
		Level zerolog.Level
		Title string
		Body  LogBody
		Color Color
	}
	RetryMessage struct {
		Message  *LogMessage
		Trials   int
		Provider Provider
	}
)

var dispatcher *Dispatcher

const retryInterval = 5 * time.Second

var maxRetries = map[zerolog.Level]int{
	zerolog.DebugLevel: 1,
	zerolog.InfoLevel:  1,
	zerolog.WarnLevel:  3,
	zerolog.ErrorLevel: 5,
	zerolog.FatalLevel: 10,
	zerolog.PanicLevel: 10,
}

func StartNotifDispatcher(parent task.Parent) *Dispatcher {
	dispatcher = &Dispatcher{
		task:        parent.Subtask("notification", true),
		providers:   F.NewSet[Provider](),
		logCh:       make(chan *LogMessage),
		retryCh:     make(chan *RetryMessage, 100),
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
		dispatcher = nil
		disp.providers.Clear()
		close(disp.logCh)
		close(disp.retryCh)
		disp.task.Finish(nil)
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
			if len(disp.retryCh) == 0 {
				continue
			}
			var msgs []*RetryMessage
			done := false
			for !done {
				select {
				case msg := <-disp.retryCh:
					msgs = append(msgs, msg)
				default:
					done = true
				}
			}
			if err := disp.retry(msgs); err != nil {
				gperr.LogError("notification retry failed", err)
			}
		}
	}
}

func (disp *Dispatcher) dispatch(msg *LogMessage) {
	task := disp.task.Subtask("dispatcher", true)
	defer task.Finish("notif dispatched")

	disp.providers.RangeAllParallel(func(p Provider) {
		if err := msg.notify(task.Context(), p); err != nil {
			disp.retryCh <- &RetryMessage{
				Message:  msg,
				Trials:   0,
				Provider: p,
			}
		}
	})
}

func (disp *Dispatcher) retry(messages []*RetryMessage) error {
	task := disp.task.Subtask("retry", true)
	defer task.Finish("notif retried")

	errs := gperr.NewBuilder("notification failure")
	for _, msg := range messages {
		err := msg.Message.notify(task.Context(), msg.Provider)
		if err == nil {
			continue
		}
		if msg.Trials > maxRetries[msg.Message.Level] {
			errs.Addf("notification provider %s failed after %d trials", msg.Provider.GetName(), msg.Trials)
			errs.Add(err)
			continue
		}
		msg.Trials++
		disp.retryCh <- msg
	}
	return errs.Error()
}

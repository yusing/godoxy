package notif

import (
	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	F "github.com/yusing/go-proxy/internal/utils/functional"
)

type (
	Dispatcher struct {
		task      *task.Task
		logCh     chan *LogMessage
		providers F.Set[Provider]
	}
	LogMessage struct {
		Level zerolog.Level
		Title string
		Body  LogBody
		Color Color
	}
)

var dispatcher *Dispatcher

const dispatchErr = "notification dispatch error"

func StartNotifDispatcher(parent task.Parent) *Dispatcher {
	dispatcher = &Dispatcher{
		task:      parent.Subtask("notification"),
		logCh:     make(chan *LogMessage),
		providers: F.NewSet[Provider](),
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

func (f *FieldsBody) Add(name, value string) {
	*f = append(*f, LogField{Name: name, Value: value})
}

func (disp *Dispatcher) RegisterProvider(cfg *NotificationConfig) {
	disp.providers.Add(cfg.Provider)
}

func (disp *Dispatcher) start() {
	defer func() {
		dispatcher = nil
		disp.providers.Clear()
		close(disp.logCh)
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
		}
	}
}

func (disp *Dispatcher) dispatch(msg *LogMessage) {
	task := disp.task.Subtask("dispatcher")
	defer task.Finish("notif dispatched")

	errs := gperr.NewBuilderWithConcurrency(dispatchErr)
	disp.providers.RangeAllParallel(func(p Provider) {
		if err := notifyProvider(task.Context(), p, msg); err != nil {
			errs.Add(gperr.PrependSubject(p.GetName(), err))
		}
	})
	if errs.HasError() {
		gperr.LogError(errs.About(), errs.Error())
	} else {
		logging.Debug().Str("title", msg.Title).Msgf("dispatched notif")
	}
}

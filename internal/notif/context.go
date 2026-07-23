package notif

import "context"

type notifierContextKey struct{}

func SetCtx(target interface{ SetValue(any, any) }, notifier Notifier) {
	target.SetValue(notifierContextKey{}, notifier)
}

func FromCtx(ctx context.Context) Notifier {
	notifier, _ := ctx.Value(notifierContextKey{}).(Notifier)
	if notifier == nil {
		return noopNotifier{}
	}
	return notifier
}

type noopNotifier struct{}

func (noopNotifier) Notify(*LogMessage) {}

package autocert

import (
	"crypto/tls"

	"github.com/yusing/godoxy/internal/task"
)

type Provider interface {
	Setup() error
	GetCert(*tls.ClientHelloInfo) (*tls.Certificate, error)
	ScheduleRenewal(task.Parent)
	ObtainCert() error
}

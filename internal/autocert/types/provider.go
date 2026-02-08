package autocert

import (
	"crypto/tls"

	"github.com/yusing/goutils/task"
)

type Provider interface {
	GetCert(*tls.ClientHelloInfo) (*tls.Certificate, error)
	ScheduleRenewalAll(task.Parent)
	ObtainCertAll() error
}

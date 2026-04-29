package autocert

import (
	"context"
	"crypto/tls"

	"github.com/yusing/goutils/task"
)

type Provider interface {
	GetCert(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
	GetCertInfos() ([]CertInfo, error)
	ScheduleRenewalAll(parent task.Parent)
	ObtainCertAll(ctx context.Context) error
	ForceExpiryAll() bool
	WaitRenewalDone(ctx context.Context) bool
}

package autocert

type CertState int

const (
	CertStateValid CertState = iota
	CertStateExpired
	CertStateMismatch
)

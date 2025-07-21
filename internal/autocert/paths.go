package autocert

const (
	certBasePath       = "certs/"
	CertFileDefault    = certBasePath + "cert.crt"
	KeyFileDefault     = certBasePath + "priv.key"
	ACMEKeyFileDefault = certBasePath + "acme.key"
	LastFailureFile    = certBasePath + ".last_failure"
)

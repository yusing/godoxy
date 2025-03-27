package certapi

import (
	"encoding/json"
	"net/http"

	config "github.com/yusing/go-proxy/internal/config/types"
)

type CertInfo struct {
	Subject        string   `json:"subject"`
	Issuer         string   `json:"issuer"`
	NotBefore      int64    `json:"not_before"`
	NotAfter       int64    `json:"not_after"`
	DNSNames       []string `json:"dns_names"`
	EmailAddresses []string `json:"email_addresses"`
}

func GetCertInfo(w http.ResponseWriter, r *http.Request) {
	autocert := config.GetInstance().AutoCertProvider()
	if autocert == nil {
		http.Error(w, "autocert is not enabled", http.StatusNotFound)
		return
	}

	cert, err := autocert.GetCert(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	certInfo := CertInfo{
		Subject:        cert.Leaf.Subject.CommonName,
		Issuer:         cert.Leaf.Issuer.CommonName,
		NotBefore:      cert.Leaf.NotBefore.Unix(),
		NotAfter:       cert.Leaf.NotAfter.Unix(),
		DNSNames:       cert.Leaf.DNSNames,
		EmailAddresses: cert.Leaf.EmailAddresses,
	}
	json.NewEncoder(w).Encode(&certInfo)
}

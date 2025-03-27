package autocert

import (
	"github.com/go-acme/lego/v4/providers/dns/clouddns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/duckdns"
	"github.com/go-acme/lego/v4/providers/dns/ovh"
	"github.com/go-acme/lego/v4/providers/dns/porkbun"
)

const (
	certBasePath       = "certs/"
	CertFileDefault    = certBasePath + "cert.crt"
	KeyFileDefault     = certBasePath + "priv.key"
	ACMEKeyFileDefault = certBasePath + "acme.key"
)

const (
	ProviderLocal      = "local"
	ProviderCloudflare = "cloudflare"
	ProviderClouddns   = "clouddns"
	ProviderDuckdns    = "duckdns"
	ProviderOVH        = "ovh"
	ProviderPseudo     = "pseudo" // for testing
	ProviderPorkbun    = "porkbun"
)

var providersGenMap = map[string]ProviderGenerator{
	ProviderLocal:      providerGenerator(NewDummyDefaultConfig, NewDummyDNSProviderConfig),
	ProviderCloudflare: providerGenerator(cloudflare.NewDefaultConfig, cloudflare.NewDNSProviderConfig),
	ProviderClouddns:   providerGenerator(clouddns.NewDefaultConfig, clouddns.NewDNSProviderConfig),
	ProviderDuckdns:    providerGenerator(duckdns.NewDefaultConfig, duckdns.NewDNSProviderConfig),
	ProviderOVH:        providerGenerator(ovh.NewDefaultConfig, ovh.NewDNSProviderConfig),
	ProviderPseudo:     providerGenerator(NewDummyDefaultConfig, NewDummyDNSProviderConfig),
	ProviderPorkbun:    providerGenerator(porkbun.NewDefaultConfig, porkbun.NewDNSProviderConfig),
}

//go:generate /usr/bin/python3 gen.py

package dnsproviders

import (
	"github.com/go-acme/lego/v4/providers/dns/acmedns"
	"github.com/go-acme/lego/v4/providers/dns/azuredns"
	"github.com/go-acme/lego/v4/providers/dns/clouddns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/cloudns"
	"github.com/go-acme/lego/v4/providers/dns/digitalocean"
	"github.com/go-acme/lego/v4/providers/dns/duckdns"
	"github.com/go-acme/lego/v4/providers/dns/edgedns"
	"github.com/go-acme/lego/v4/providers/dns/gcloud"
	"github.com/go-acme/lego/v4/providers/dns/godaddy"
	"github.com/go-acme/lego/v4/providers/dns/googledomains"
	"github.com/go-acme/lego/v4/providers/dns/hetzner"
	"github.com/go-acme/lego/v4/providers/dns/httpreq"
	"github.com/go-acme/lego/v4/providers/dns/ionos"
	"github.com/go-acme/lego/v4/providers/dns/linode"
	"github.com/go-acme/lego/v4/providers/dns/namecheap"
	"github.com/go-acme/lego/v4/providers/dns/netcup"
	"github.com/go-acme/lego/v4/providers/dns/netlify"
	"github.com/go-acme/lego/v4/providers/dns/oraclecloud"
	"github.com/go-acme/lego/v4/providers/dns/ovh"
	"github.com/go-acme/lego/v4/providers/dns/porkbun"
	"github.com/go-acme/lego/v4/providers/dns/rfc2136"
	"github.com/go-acme/lego/v4/providers/dns/scaleway"
	"github.com/go-acme/lego/v4/providers/dns/spaceship"
	"github.com/go-acme/lego/v4/providers/dns/timewebcloud"
	"github.com/go-acme/lego/v4/providers/dns/vercel"
	"github.com/go-acme/lego/v4/providers/dns/vultr"
	"github.com/yusing/godoxy/internal/autocert"
)

const (
	Local  = "local"
	Pseudo = "pseudo"
)

func InitProviders() {
	autocert.Providers[Local] = autocert.DNSProvider(NewDummyDefaultConfig, NewDummyDNSProviderConfig)
	autocert.Providers[Pseudo] = autocert.DNSProvider(NewDummyDefaultConfig, NewDummyDNSProviderConfig)
	autocert.Providers["acmedns"] = autocert.DNSProvider(acmedns.NewDefaultConfig, acmedns.NewDNSProviderConfig)
	autocert.Providers["azuredns"] = autocert.DNSProvider(azuredns.NewDefaultConfig, azuredns.NewDNSProviderConfig)
	autocert.Providers["cloudflare"] = autocert.DNSProvider(cloudflare.NewDefaultConfig, cloudflare.NewDNSProviderConfig)
	autocert.Providers["cloudns"] = autocert.DNSProvider(cloudns.NewDefaultConfig, cloudns.NewDNSProviderConfig)
	autocert.Providers["clouddns"] = autocert.DNSProvider(clouddns.NewDefaultConfig, clouddns.NewDNSProviderConfig)
	autocert.Providers["digitalocean"] = autocert.DNSProvider(digitalocean.NewDefaultConfig, digitalocean.NewDNSProviderConfig)
	autocert.Providers["duckdns"] = autocert.DNSProvider(duckdns.NewDefaultConfig, duckdns.NewDNSProviderConfig)
	autocert.Providers["edgedns"] = autocert.DNSProvider(edgedns.NewDefaultConfig, edgedns.NewDNSProviderConfig)
	autocert.Providers["gcloud"] = autocert.DNSProvider(gcloud.NewDefaultConfig, gcloud.NewDNSProviderConfig)
	autocert.Providers["godaddy"] = autocert.DNSProvider(godaddy.NewDefaultConfig, godaddy.NewDNSProviderConfig)
	autocert.Providers["googledomains"] = autocert.DNSProvider(googledomains.NewDefaultConfig, googledomains.NewDNSProviderConfig)
	autocert.Providers["hetzner"] = autocert.DNSProvider(hetzner.NewDefaultConfig, hetzner.NewDNSProviderConfig)
	autocert.Providers["httpreq"] = autocert.DNSProvider(httpreq.NewDefaultConfig, httpreq.NewDNSProviderConfig)
	autocert.Providers["ionos"] = autocert.DNSProvider(ionos.NewDefaultConfig, ionos.NewDNSProviderConfig)
	autocert.Providers["linode"] = autocert.DNSProvider(linode.NewDefaultConfig, linode.NewDNSProviderConfig)
	autocert.Providers["namecheap"] = autocert.DNSProvider(namecheap.NewDefaultConfig, namecheap.NewDNSProviderConfig)
	autocert.Providers["netcup"] = autocert.DNSProvider(netcup.NewDefaultConfig, netcup.NewDNSProviderConfig)
	autocert.Providers["netlify"] = autocert.DNSProvider(netlify.NewDefaultConfig, netlify.NewDNSProviderConfig)
	autocert.Providers["oraclecloud"] = autocert.DNSProvider(oraclecloud.NewDefaultConfig, oraclecloud.NewDNSProviderConfig)
	autocert.Providers["ovh"] = autocert.DNSProvider(ovh.NewDefaultConfig, ovh.NewDNSProviderConfig)
	autocert.Providers["porkbun"] = autocert.DNSProvider(porkbun.NewDefaultConfig, porkbun.NewDNSProviderConfig)
	autocert.Providers["rfc2136"] = autocert.DNSProvider(rfc2136.NewDefaultConfig, rfc2136.NewDNSProviderConfig)
	autocert.Providers["scaleway"] = autocert.DNSProvider(scaleway.NewDefaultConfig, scaleway.NewDNSProviderConfig)
	autocert.Providers["spaceship"] = autocert.DNSProvider(spaceship.NewDefaultConfig, spaceship.NewDNSProviderConfig)
	autocert.Providers["vercel"] = autocert.DNSProvider(vercel.NewDefaultConfig, vercel.NewDNSProviderConfig)
	autocert.Providers["vultr"] = autocert.DNSProvider(vultr.NewDefaultConfig, vultr.NewDNSProviderConfig)
	autocert.Providers["timewebcloud"] = autocert.DNSProvider(timewebcloud.NewDefaultConfig, timewebcloud.NewDNSProviderConfig)
}

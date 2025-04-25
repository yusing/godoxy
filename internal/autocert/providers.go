//go:generate /usr/bin/python3 gen.py

package autocert

import "github.com/go-acme/lego/v4/providers/dns/acmedns"
import "github.com/go-acme/lego/v4/providers/dns/active24"
import "github.com/go-acme/lego/v4/providers/dns/alidns"
import "github.com/go-acme/lego/v4/providers/dns/allinkl"
import "github.com/go-acme/lego/v4/providers/dns/arvancloud"
import "github.com/go-acme/lego/v4/providers/dns/auroradns"
import "github.com/go-acme/lego/v4/providers/dns/autodns"
import "github.com/go-acme/lego/v4/providers/dns/axelname"
import "github.com/go-acme/lego/v4/providers/dns/azuredns"
import "github.com/go-acme/lego/v4/providers/dns/baiducloud"
import "github.com/go-acme/lego/v4/providers/dns/bindman"
import "github.com/go-acme/lego/v4/providers/dns/bluecat"
import "github.com/go-acme/lego/v4/providers/dns/bookmyname"
import "github.com/go-acme/lego/v4/providers/dns/bunny"
import "github.com/go-acme/lego/v4/providers/dns/checkdomain"
import "github.com/go-acme/lego/v4/providers/dns/civo"
import "github.com/go-acme/lego/v4/providers/dns/clouddns"
import "github.com/go-acme/lego/v4/providers/dns/cloudflare"
import "github.com/go-acme/lego/v4/providers/dns/cloudns"
import "github.com/go-acme/lego/v4/providers/dns/cloudru"
import "github.com/go-acme/lego/v4/providers/dns/conoha"
import "github.com/go-acme/lego/v4/providers/dns/constellix"
import "github.com/go-acme/lego/v4/providers/dns/corenetworks"
import "github.com/go-acme/lego/v4/providers/dns/cpanel"
import "github.com/go-acme/lego/v4/providers/dns/derak"
import "github.com/go-acme/lego/v4/providers/dns/desec"
import "github.com/go-acme/lego/v4/providers/dns/designate"
import "github.com/go-acme/lego/v4/providers/dns/digitalocean"
import "github.com/go-acme/lego/v4/providers/dns/directadmin"
import "github.com/go-acme/lego/v4/providers/dns/dnshomede"
import "github.com/go-acme/lego/v4/providers/dns/dnsimple"
import "github.com/go-acme/lego/v4/providers/dns/dnsmadeeasy"
import "github.com/go-acme/lego/v4/providers/dns/dode"
import "github.com/go-acme/lego/v4/providers/dns/domeneshop"
import "github.com/go-acme/lego/v4/providers/dns/dreamhost"
import "github.com/go-acme/lego/v4/providers/dns/duckdns"
import "github.com/go-acme/lego/v4/providers/dns/dyn"
import "github.com/go-acme/lego/v4/providers/dns/dynu"
import "github.com/go-acme/lego/v4/providers/dns/easydns"
import "github.com/go-acme/lego/v4/providers/dns/edgedns"
import "github.com/go-acme/lego/v4/providers/dns/efficientip"
import "github.com/go-acme/lego/v4/providers/dns/epik"
import "github.com/go-acme/lego/v4/providers/dns/exec"
import "github.com/go-acme/lego/v4/providers/dns/exoscale"
import "github.com/go-acme/lego/v4/providers/dns/f5xc"
import "github.com/go-acme/lego/v4/providers/dns/freemyip"
import "github.com/go-acme/lego/v4/providers/dns/gandi"
import "github.com/go-acme/lego/v4/providers/dns/gandiv5"
import "github.com/go-acme/lego/v4/providers/dns/gcloud"
import "github.com/go-acme/lego/v4/providers/dns/gcore"
import "github.com/go-acme/lego/v4/providers/dns/glesys"
import "github.com/go-acme/lego/v4/providers/dns/godaddy"
import "github.com/go-acme/lego/v4/providers/dns/googledomains"
import "github.com/go-acme/lego/v4/providers/dns/hetzner"
import "github.com/go-acme/lego/v4/providers/dns/hostingde"
import "github.com/go-acme/lego/v4/providers/dns/hosttech"
import "github.com/go-acme/lego/v4/providers/dns/httpnet"
import "github.com/go-acme/lego/v4/providers/dns/httpreq"
import "github.com/go-acme/lego/v4/providers/dns/huaweicloud"
import "github.com/go-acme/lego/v4/providers/dns/hurricane"
import "github.com/go-acme/lego/v4/providers/dns/hyperone"
import "github.com/go-acme/lego/v4/providers/dns/ibmcloud"
import "github.com/go-acme/lego/v4/providers/dns/iij"
import "github.com/go-acme/lego/v4/providers/dns/iijdpf"
import "github.com/go-acme/lego/v4/providers/dns/infoblox"
import "github.com/go-acme/lego/v4/providers/dns/infomaniak"
import "github.com/go-acme/lego/v4/providers/dns/internetbs"
import "github.com/go-acme/lego/v4/providers/dns/inwx"
import "github.com/go-acme/lego/v4/providers/dns/ionos"
import "github.com/go-acme/lego/v4/providers/dns/ipv64"
import "github.com/go-acme/lego/v4/providers/dns/iwantmyname"
import "github.com/go-acme/lego/v4/providers/dns/joker"
import "github.com/go-acme/lego/v4/providers/dns/liara"
import "github.com/go-acme/lego/v4/providers/dns/lightsail"
import "github.com/go-acme/lego/v4/providers/dns/limacity"
import "github.com/go-acme/lego/v4/providers/dns/linode"
import "github.com/go-acme/lego/v4/providers/dns/liquidweb"
import "github.com/go-acme/lego/v4/providers/dns/loopia"
import "github.com/go-acme/lego/v4/providers/dns/luadns"
import "github.com/go-acme/lego/v4/providers/dns/mailinabox"
import "github.com/go-acme/lego/v4/providers/dns/manageengine"
import "github.com/go-acme/lego/v4/providers/dns/metaname"
import "github.com/go-acme/lego/v4/providers/dns/metaregistrar"
import "github.com/go-acme/lego/v4/providers/dns/mijnhost"
import "github.com/go-acme/lego/v4/providers/dns/mittwald"
import "github.com/go-acme/lego/v4/providers/dns/myaddr"
import "github.com/go-acme/lego/v4/providers/dns/mydnsjp"
import "github.com/go-acme/lego/v4/providers/dns/namecheap"
import "github.com/go-acme/lego/v4/providers/dns/namedotcom"
import "github.com/go-acme/lego/v4/providers/dns/namesilo"
import "github.com/go-acme/lego/v4/providers/dns/nearlyfreespeech"
import "github.com/go-acme/lego/v4/providers/dns/netcup"
import "github.com/go-acme/lego/v4/providers/dns/netlify"
import "github.com/go-acme/lego/v4/providers/dns/nicmanager"
import "github.com/go-acme/lego/v4/providers/dns/nifcloud"
import "github.com/go-acme/lego/v4/providers/dns/njalla"
import "github.com/go-acme/lego/v4/providers/dns/nodion"
import "github.com/go-acme/lego/v4/providers/dns/ns1"
import "github.com/go-acme/lego/v4/providers/dns/oraclecloud"
import "github.com/go-acme/lego/v4/providers/dns/otc"
import "github.com/go-acme/lego/v4/providers/dns/ovh"
import "github.com/go-acme/lego/v4/providers/dns/pdns"
import "github.com/go-acme/lego/v4/providers/dns/plesk"
import "github.com/go-acme/lego/v4/providers/dns/porkbun"
import "github.com/go-acme/lego/v4/providers/dns/rackspace"
import "github.com/go-acme/lego/v4/providers/dns/rainyun"
import "github.com/go-acme/lego/v4/providers/dns/rcodezero"
import "github.com/go-acme/lego/v4/providers/dns/regfish"
import "github.com/go-acme/lego/v4/providers/dns/regru"
import "github.com/go-acme/lego/v4/providers/dns/rfc2136"
import "github.com/go-acme/lego/v4/providers/dns/rimuhosting"
import "github.com/go-acme/lego/v4/providers/dns/route53"
import "github.com/go-acme/lego/v4/providers/dns/safedns"
import "github.com/go-acme/lego/v4/providers/dns/sakuracloud"
import "github.com/go-acme/lego/v4/providers/dns/scaleway"
import "github.com/go-acme/lego/v4/providers/dns/selectel"
import "github.com/go-acme/lego/v4/providers/dns/selectelv2"
import "github.com/go-acme/lego/v4/providers/dns/selfhostde"
import "github.com/go-acme/lego/v4/providers/dns/servercow"
import "github.com/go-acme/lego/v4/providers/dns/shellrent"
import "github.com/go-acme/lego/v4/providers/dns/simply"
import "github.com/go-acme/lego/v4/providers/dns/sonic"
import "github.com/go-acme/lego/v4/providers/dns/spaceship"
import "github.com/go-acme/lego/v4/providers/dns/stackpath"
import "github.com/go-acme/lego/v4/providers/dns/technitium"
import "github.com/go-acme/lego/v4/providers/dns/tencentcloud"
import "github.com/go-acme/lego/v4/providers/dns/timewebcloud"
import "github.com/go-acme/lego/v4/providers/dns/transip"
import "github.com/go-acme/lego/v4/providers/dns/ultradns"
import "github.com/go-acme/lego/v4/providers/dns/variomedia"
import "github.com/go-acme/lego/v4/providers/dns/vegadns"
import "github.com/go-acme/lego/v4/providers/dns/vercel"
import "github.com/go-acme/lego/v4/providers/dns/versio"
import "github.com/go-acme/lego/v4/providers/dns/vinyldns"
import "github.com/go-acme/lego/v4/providers/dns/vkcloud"
import "github.com/go-acme/lego/v4/providers/dns/volcengine"
import "github.com/go-acme/lego/v4/providers/dns/vscale"
import "github.com/go-acme/lego/v4/providers/dns/vultr"
import "github.com/go-acme/lego/v4/providers/dns/webnames"
import "github.com/go-acme/lego/v4/providers/dns/websupport"
import "github.com/go-acme/lego/v4/providers/dns/wedos"
import "github.com/go-acme/lego/v4/providers/dns/westcn"
import "github.com/go-acme/lego/v4/providers/dns/yandex"
import "github.com/go-acme/lego/v4/providers/dns/yandex360"
import "github.com/go-acme/lego/v4/providers/dns/zoneee"
import "github.com/go-acme/lego/v4/providers/dns/zonomi"

const (
	ProviderLocal            = "local"
	ProviderPseudo           = "pseudo"
	Provideracmedns          = "acmedns"
	Provideractive24         = "active24"
	Provideralidns           = "alidns"
	Providerallinkl          = "allinkl"
	Providerarvancloud       = "arvancloud"
	Providerauroradns        = "auroradns"
	Providerautodns          = "autodns"
	Provideraxelname         = "axelname"
	Providerazuredns         = "azuredns"
	Providerbaiducloud       = "baiducloud"
	Providerbindman          = "bindman"
	Providerbluecat          = "bluecat"
	Providerbookmyname       = "bookmyname"
	Providerbunny            = "bunny"
	Providercheckdomain      = "checkdomain"
	Providercivo             = "civo"
	Providerclouddns         = "clouddns"
	Providercloudflare       = "cloudflare"
	Providercloudns          = "cloudns"
	Providercloudru          = "cloudru"
	Providerconoha           = "conoha"
	Providerconstellix       = "constellix"
	Providercorenetworks     = "corenetworks"
	Providercpanel           = "cpanel"
	Providerderak            = "derak"
	Providerdesec            = "desec"
	Providerdesignate        = "designate"
	Providerdigitalocean     = "digitalocean"
	Providerdirectadmin      = "directadmin"
	Providerdnshomede        = "dnshomede"
	Providerdnsimple         = "dnsimple"
	Providerdnsmadeeasy      = "dnsmadeeasy"
	Providerdode             = "dode"
	Providerdomeneshop       = "domeneshop"
	Providerdreamhost        = "dreamhost"
	Providerduckdns          = "duckdns"
	Providerdyn              = "dyn"
	Providerdynu             = "dynu"
	Providereasydns          = "easydns"
	Provideredgedns          = "edgedns"
	Providerefficientip      = "efficientip"
	Providerepik             = "epik"
	Providerexec             = "exec"
	Providerexoscale         = "exoscale"
	Providerf5xc             = "f5xc"
	Providerfreemyip         = "freemyip"
	Providergandi            = "gandi"
	Providergandiv5          = "gandiv5"
	Providergcloud           = "gcloud"
	Providergcore            = "gcore"
	Providerglesys           = "glesys"
	Providergodaddy          = "godaddy"
	Providergoogledomains    = "googledomains"
	Providerhetzner          = "hetzner"
	Providerhostingde        = "hostingde"
	Providerhosttech         = "hosttech"
	Providerhttpnet          = "httpnet"
	Providerhttpreq          = "httpreq"
	Providerhuaweicloud      = "huaweicloud"
	Providerhurricane        = "hurricane"
	Providerhyperone         = "hyperone"
	Provideribmcloud         = "ibmcloud"
	Provideriij              = "iij"
	Provideriijdpf           = "iijdpf"
	Providerinfoblox         = "infoblox"
	Providerinfomaniak       = "infomaniak"
	Providerinternetbs       = "internetbs"
	Providerinwx             = "inwx"
	Providerionos            = "ionos"
	Provideripv64            = "ipv64"
	Provideriwantmyname      = "iwantmyname"
	Providerjoker            = "joker"
	Providerliara            = "liara"
	Providerlightsail        = "lightsail"
	Providerlimacity         = "limacity"
	Providerlinode           = "linode"
	Providerliquidweb        = "liquidweb"
	Providerloopia           = "loopia"
	Providerluadns           = "luadns"
	Providermailinabox       = "mailinabox"
	Providermanageengine     = "manageengine"
	Providermetaname         = "metaname"
	Providermetaregistrar    = "metaregistrar"
	Providermijnhost         = "mijnhost"
	Providermittwald         = "mittwald"
	Providermyaddr           = "myaddr"
	Providermydnsjp          = "mydnsjp"
	Providernamecheap        = "namecheap"
	Providernamedotcom       = "namedotcom"
	Providernamesilo         = "namesilo"
	Providernearlyfreespeech = "nearlyfreespeech"
	Providernetcup           = "netcup"
	Providernetlify          = "netlify"
	Providernicmanager       = "nicmanager"
	Providernifcloud         = "nifcloud"
	Providernjalla           = "njalla"
	Providernodion           = "nodion"
	Providerns1              = "ns1"
	Provideroraclecloud      = "oraclecloud"
	Providerotc              = "otc"
	Providerovh              = "ovh"
	Providerpdns             = "pdns"
	Providerplesk            = "plesk"
	Providerporkbun          = "porkbun"
	Providerrackspace        = "rackspace"
	Providerrainyun          = "rainyun"
	Providerrcodezero        = "rcodezero"
	Providerregfish          = "regfish"
	Providerregru            = "regru"
	Providerrfc2136          = "rfc2136"
	Providerrimuhosting      = "rimuhosting"
	Providerroute53          = "route53"
	Providersafedns          = "safedns"
	Providersakuracloud      = "sakuracloud"
	Providerscaleway         = "scaleway"
	Providerselectel         = "selectel"
	Providerselectelv2       = "selectelv2"
	Providerselfhostde       = "selfhostde"
	Providerservercow        = "servercow"
	Providershellrent        = "shellrent"
	Providersimply           = "simply"
	Providersonic            = "sonic"
	Providerspaceship        = "spaceship"
	Providerstackpath        = "stackpath"
	Providertechnitium       = "technitium"
	Providertencentcloud     = "tencentcloud"
	Providertimewebcloud     = "timewebcloud"
	Providertransip          = "transip"
	Providerultradns         = "ultradns"
	Providervariomedia       = "variomedia"
	Providervegadns          = "vegadns"
	Providervercel           = "vercel"
	Providerversio           = "versio"
	Providervinyldns         = "vinyldns"
	Providervkcloud          = "vkcloud"
	Providervolcengine       = "volcengine"
	Providervscale           = "vscale"
	Providervultr            = "vultr"
	Providerwebnames         = "webnames"
	Providerwebsupport       = "websupport"
	Providerwedos            = "wedos"
	Providerwestcn           = "westcn"
	Provideryandex           = "yandex"
	Provideryandex360        = "yandex360"
	Providerzoneee           = "zoneee"
	Providerzonomi           = "zonomi"
)

var providers = map[string]ProviderGenerator{
	ProviderLocal:            providerGenerator(NewDummyDefaultConfig, NewDummyDNSProviderConfig),
	ProviderPseudo:           providerGenerator(NewDummyDefaultConfig, NewDummyDNSProviderConfig),
	Provideracmedns:          providerGenerator(acmedns.NewDefaultConfig, acmedns.NewDNSProviderConfig),
	Provideractive24:         providerGenerator(active24.NewDefaultConfig, active24.NewDNSProviderConfig),
	Provideralidns:           providerGenerator(alidns.NewDefaultConfig, alidns.NewDNSProviderConfig),
	Providerallinkl:          providerGenerator(allinkl.NewDefaultConfig, allinkl.NewDNSProviderConfig),
	Providerarvancloud:       providerGenerator(arvancloud.NewDefaultConfig, arvancloud.NewDNSProviderConfig),
	Providerauroradns:        providerGenerator(auroradns.NewDefaultConfig, auroradns.NewDNSProviderConfig),
	Providerautodns:          providerGenerator(autodns.NewDefaultConfig, autodns.NewDNSProviderConfig),
	Provideraxelname:         providerGenerator(axelname.NewDefaultConfig, axelname.NewDNSProviderConfig),
	Providerazuredns:         providerGenerator(azuredns.NewDefaultConfig, azuredns.NewDNSProviderConfig),
	Providerbaiducloud:       providerGenerator(baiducloud.NewDefaultConfig, baiducloud.NewDNSProviderConfig),
	Providerbindman:          providerGenerator(bindman.NewDefaultConfig, bindman.NewDNSProviderConfig),
	Providerbluecat:          providerGenerator(bluecat.NewDefaultConfig, bluecat.NewDNSProviderConfig),
	Providerbookmyname:       providerGenerator(bookmyname.NewDefaultConfig, bookmyname.NewDNSProviderConfig),
	Providerbunny:            providerGenerator(bunny.NewDefaultConfig, bunny.NewDNSProviderConfig),
	Providercheckdomain:      providerGenerator(checkdomain.NewDefaultConfig, checkdomain.NewDNSProviderConfig),
	Providercivo:             providerGenerator(civo.NewDefaultConfig, civo.NewDNSProviderConfig),
	Providerclouddns:         providerGenerator(clouddns.NewDefaultConfig, clouddns.NewDNSProviderConfig),
	Providercloudflare:       providerGenerator(cloudflare.NewDefaultConfig, cloudflare.NewDNSProviderConfig),
	Providercloudns:          providerGenerator(cloudns.NewDefaultConfig, cloudns.NewDNSProviderConfig),
	Providercloudru:          providerGenerator(cloudru.NewDefaultConfig, cloudru.NewDNSProviderConfig),
	Providerconoha:           providerGenerator(conoha.NewDefaultConfig, conoha.NewDNSProviderConfig),
	Providerconstellix:       providerGenerator(constellix.NewDefaultConfig, constellix.NewDNSProviderConfig),
	Providercorenetworks:     providerGenerator(corenetworks.NewDefaultConfig, corenetworks.NewDNSProviderConfig),
	Providercpanel:           providerGenerator(cpanel.NewDefaultConfig, cpanel.NewDNSProviderConfig),
	Providerderak:            providerGenerator(derak.NewDefaultConfig, derak.NewDNSProviderConfig),
	Providerdesec:            providerGenerator(desec.NewDefaultConfig, desec.NewDNSProviderConfig),
	Providerdesignate:        providerGenerator(designate.NewDefaultConfig, designate.NewDNSProviderConfig),
	Providerdigitalocean:     providerGenerator(digitalocean.NewDefaultConfig, digitalocean.NewDNSProviderConfig),
	Providerdirectadmin:      providerGenerator(directadmin.NewDefaultConfig, directadmin.NewDNSProviderConfig),
	Providerdnshomede:        providerGenerator(dnshomede.NewDefaultConfig, dnshomede.NewDNSProviderConfig),
	Providerdnsimple:         providerGenerator(dnsimple.NewDefaultConfig, dnsimple.NewDNSProviderConfig),
	Providerdnsmadeeasy:      providerGenerator(dnsmadeeasy.NewDefaultConfig, dnsmadeeasy.NewDNSProviderConfig),
	Providerdode:             providerGenerator(dode.NewDefaultConfig, dode.NewDNSProviderConfig),
	Providerdomeneshop:       providerGenerator(domeneshop.NewDefaultConfig, domeneshop.NewDNSProviderConfig),
	Providerdreamhost:        providerGenerator(dreamhost.NewDefaultConfig, dreamhost.NewDNSProviderConfig),
	Providerduckdns:          providerGenerator(duckdns.NewDefaultConfig, duckdns.NewDNSProviderConfig),
	Providerdyn:              providerGenerator(dyn.NewDefaultConfig, dyn.NewDNSProviderConfig),
	Providerdynu:             providerGenerator(dynu.NewDefaultConfig, dynu.NewDNSProviderConfig),
	Providereasydns:          providerGenerator(easydns.NewDefaultConfig, easydns.NewDNSProviderConfig),
	Provideredgedns:          providerGenerator(edgedns.NewDefaultConfig, edgedns.NewDNSProviderConfig),
	Providerefficientip:      providerGenerator(efficientip.NewDefaultConfig, efficientip.NewDNSProviderConfig),
	Providerepik:             providerGenerator(epik.NewDefaultConfig, epik.NewDNSProviderConfig),
	Providerexec:             providerGenerator(exec.NewDefaultConfig, exec.NewDNSProviderConfig),
	Providerexoscale:         providerGenerator(exoscale.NewDefaultConfig, exoscale.NewDNSProviderConfig),
	Providerf5xc:             providerGenerator(f5xc.NewDefaultConfig, f5xc.NewDNSProviderConfig),
	Providerfreemyip:         providerGenerator(freemyip.NewDefaultConfig, freemyip.NewDNSProviderConfig),
	Providergandi:            providerGenerator(gandi.NewDefaultConfig, gandi.NewDNSProviderConfig),
	Providergandiv5:          providerGenerator(gandiv5.NewDefaultConfig, gandiv5.NewDNSProviderConfig),
	Providergcloud:           providerGenerator(gcloud.NewDefaultConfig, gcloud.NewDNSProviderConfig),
	Providergcore:            providerGenerator(gcore.NewDefaultConfig, gcore.NewDNSProviderConfig),
	Providerglesys:           providerGenerator(glesys.NewDefaultConfig, glesys.NewDNSProviderConfig),
	Providergodaddy:          providerGenerator(godaddy.NewDefaultConfig, godaddy.NewDNSProviderConfig),
	Providergoogledomains:    providerGenerator(googledomains.NewDefaultConfig, googledomains.NewDNSProviderConfig),
	Providerhetzner:          providerGenerator(hetzner.NewDefaultConfig, hetzner.NewDNSProviderConfig),
	Providerhostingde:        providerGenerator(hostingde.NewDefaultConfig, hostingde.NewDNSProviderConfig),
	Providerhosttech:         providerGenerator(hosttech.NewDefaultConfig, hosttech.NewDNSProviderConfig),
	Providerhttpnet:          providerGenerator(httpnet.NewDefaultConfig, httpnet.NewDNSProviderConfig),
	Providerhttpreq:          providerGenerator(httpreq.NewDefaultConfig, httpreq.NewDNSProviderConfig),
	Providerhuaweicloud:      providerGenerator(huaweicloud.NewDefaultConfig, huaweicloud.NewDNSProviderConfig),
	Providerhurricane:        providerGenerator(hurricane.NewDefaultConfig, hurricane.NewDNSProviderConfig),
	Providerhyperone:         providerGenerator(hyperone.NewDefaultConfig, hyperone.NewDNSProviderConfig),
	Provideribmcloud:         providerGenerator(ibmcloud.NewDefaultConfig, ibmcloud.NewDNSProviderConfig),
	Provideriij:              providerGenerator(iij.NewDefaultConfig, iij.NewDNSProviderConfig),
	Provideriijdpf:           providerGenerator(iijdpf.NewDefaultConfig, iijdpf.NewDNSProviderConfig),
	Providerinfoblox:         providerGenerator(infoblox.NewDefaultConfig, infoblox.NewDNSProviderConfig),
	Providerinfomaniak:       providerGenerator(infomaniak.NewDefaultConfig, infomaniak.NewDNSProviderConfig),
	Providerinternetbs:       providerGenerator(internetbs.NewDefaultConfig, internetbs.NewDNSProviderConfig),
	Providerinwx:             providerGenerator(inwx.NewDefaultConfig, inwx.NewDNSProviderConfig),
	Providerionos:            providerGenerator(ionos.NewDefaultConfig, ionos.NewDNSProviderConfig),
	Provideripv64:            providerGenerator(ipv64.NewDefaultConfig, ipv64.NewDNSProviderConfig),
	Provideriwantmyname:      providerGenerator(iwantmyname.NewDefaultConfig, iwantmyname.NewDNSProviderConfig),
	Providerjoker:            providerGenerator(joker.NewDefaultConfig, joker.NewDNSProviderConfig),
	Providerliara:            providerGenerator(liara.NewDefaultConfig, liara.NewDNSProviderConfig),
	Providerlightsail:        providerGenerator(lightsail.NewDefaultConfig, lightsail.NewDNSProviderConfig),
	Providerlimacity:         providerGenerator(limacity.NewDefaultConfig, limacity.NewDNSProviderConfig),
	Providerlinode:           providerGenerator(linode.NewDefaultConfig, linode.NewDNSProviderConfig),
	Providerliquidweb:        providerGenerator(liquidweb.NewDefaultConfig, liquidweb.NewDNSProviderConfig),
	Providerloopia:           providerGenerator(loopia.NewDefaultConfig, loopia.NewDNSProviderConfig),
	Providerluadns:           providerGenerator(luadns.NewDefaultConfig, luadns.NewDNSProviderConfig),
	Providermailinabox:       providerGenerator(mailinabox.NewDefaultConfig, mailinabox.NewDNSProviderConfig),
	Providermanageengine:     providerGenerator(manageengine.NewDefaultConfig, manageengine.NewDNSProviderConfig),
	Providermetaname:         providerGenerator(metaname.NewDefaultConfig, metaname.NewDNSProviderConfig),
	Providermetaregistrar:    providerGenerator(metaregistrar.NewDefaultConfig, metaregistrar.NewDNSProviderConfig),
	Providermijnhost:         providerGenerator(mijnhost.NewDefaultConfig, mijnhost.NewDNSProviderConfig),
	Providermittwald:         providerGenerator(mittwald.NewDefaultConfig, mittwald.NewDNSProviderConfig),
	Providermyaddr:           providerGenerator(myaddr.NewDefaultConfig, myaddr.NewDNSProviderConfig),
	Providermydnsjp:          providerGenerator(mydnsjp.NewDefaultConfig, mydnsjp.NewDNSProviderConfig),
	Providernamecheap:        providerGenerator(namecheap.NewDefaultConfig, namecheap.NewDNSProviderConfig),
	Providernamedotcom:       providerGenerator(namedotcom.NewDefaultConfig, namedotcom.NewDNSProviderConfig),
	Providernamesilo:         providerGenerator(namesilo.NewDefaultConfig, namesilo.NewDNSProviderConfig),
	Providernearlyfreespeech: providerGenerator(nearlyfreespeech.NewDefaultConfig, nearlyfreespeech.NewDNSProviderConfig),
	Providernetcup:           providerGenerator(netcup.NewDefaultConfig, netcup.NewDNSProviderConfig),
	Providernetlify:          providerGenerator(netlify.NewDefaultConfig, netlify.NewDNSProviderConfig),
	Providernicmanager:       providerGenerator(nicmanager.NewDefaultConfig, nicmanager.NewDNSProviderConfig),
	Providernifcloud:         providerGenerator(nifcloud.NewDefaultConfig, nifcloud.NewDNSProviderConfig),
	Providernjalla:           providerGenerator(njalla.NewDefaultConfig, njalla.NewDNSProviderConfig),
	Providernodion:           providerGenerator(nodion.NewDefaultConfig, nodion.NewDNSProviderConfig),
	Providerns1:              providerGenerator(ns1.NewDefaultConfig, ns1.NewDNSProviderConfig),
	Provideroraclecloud:      providerGenerator(oraclecloud.NewDefaultConfig, oraclecloud.NewDNSProviderConfig),
	Providerotc:              providerGenerator(otc.NewDefaultConfig, otc.NewDNSProviderConfig),
	Providerovh:              providerGenerator(ovh.NewDefaultConfig, ovh.NewDNSProviderConfig),
	Providerpdns:             providerGenerator(pdns.NewDefaultConfig, pdns.NewDNSProviderConfig),
	Providerplesk:            providerGenerator(plesk.NewDefaultConfig, plesk.NewDNSProviderConfig),
	Providerporkbun:          providerGenerator(porkbun.NewDefaultConfig, porkbun.NewDNSProviderConfig),
	Providerrackspace:        providerGenerator(rackspace.NewDefaultConfig, rackspace.NewDNSProviderConfig),
	Providerrainyun:          providerGenerator(rainyun.NewDefaultConfig, rainyun.NewDNSProviderConfig),
	Providerrcodezero:        providerGenerator(rcodezero.NewDefaultConfig, rcodezero.NewDNSProviderConfig),
	Providerregfish:          providerGenerator(regfish.NewDefaultConfig, regfish.NewDNSProviderConfig),
	Providerregru:            providerGenerator(regru.NewDefaultConfig, regru.NewDNSProviderConfig),
	Providerrfc2136:          providerGenerator(rfc2136.NewDefaultConfig, rfc2136.NewDNSProviderConfig),
	Providerrimuhosting:      providerGenerator(rimuhosting.NewDefaultConfig, rimuhosting.NewDNSProviderConfig),
	Providerroute53:          providerGenerator(route53.NewDefaultConfig, route53.NewDNSProviderConfig),
	Providersafedns:          providerGenerator(safedns.NewDefaultConfig, safedns.NewDNSProviderConfig),
	Providersakuracloud:      providerGenerator(sakuracloud.NewDefaultConfig, sakuracloud.NewDNSProviderConfig),
	Providerscaleway:         providerGenerator(scaleway.NewDefaultConfig, scaleway.NewDNSProviderConfig),
	Providerselectel:         providerGenerator(selectel.NewDefaultConfig, selectel.NewDNSProviderConfig),
	Providerselectelv2:       providerGenerator(selectelv2.NewDefaultConfig, selectelv2.NewDNSProviderConfig),
	Providerselfhostde:       providerGenerator(selfhostde.NewDefaultConfig, selfhostde.NewDNSProviderConfig),
	Providerservercow:        providerGenerator(servercow.NewDefaultConfig, servercow.NewDNSProviderConfig),
	Providershellrent:        providerGenerator(shellrent.NewDefaultConfig, shellrent.NewDNSProviderConfig),
	Providersimply:           providerGenerator(simply.NewDefaultConfig, simply.NewDNSProviderConfig),
	Providersonic:            providerGenerator(sonic.NewDefaultConfig, sonic.NewDNSProviderConfig),
	Providerspaceship:        providerGenerator(spaceship.NewDefaultConfig, spaceship.NewDNSProviderConfig),
	Providerstackpath:        providerGenerator(stackpath.NewDefaultConfig, stackpath.NewDNSProviderConfig),
	Providertechnitium:       providerGenerator(technitium.NewDefaultConfig, technitium.NewDNSProviderConfig),
	Providertencentcloud:     providerGenerator(tencentcloud.NewDefaultConfig, tencentcloud.NewDNSProviderConfig),
	Providertimewebcloud:     providerGenerator(timewebcloud.NewDefaultConfig, timewebcloud.NewDNSProviderConfig),
	Providertransip:          providerGenerator(transip.NewDefaultConfig, transip.NewDNSProviderConfig),
	Providerultradns:         providerGenerator(ultradns.NewDefaultConfig, ultradns.NewDNSProviderConfig),
	Providervariomedia:       providerGenerator(variomedia.NewDefaultConfig, variomedia.NewDNSProviderConfig),
	Providervegadns:          providerGenerator(vegadns.NewDefaultConfig, vegadns.NewDNSProviderConfig),
	Providervercel:           providerGenerator(vercel.NewDefaultConfig, vercel.NewDNSProviderConfig),
	Providerversio:           providerGenerator(versio.NewDefaultConfig, versio.NewDNSProviderConfig),
	Providervinyldns:         providerGenerator(vinyldns.NewDefaultConfig, vinyldns.NewDNSProviderConfig),
	Providervkcloud:          providerGenerator(vkcloud.NewDefaultConfig, vkcloud.NewDNSProviderConfig),
	Providervolcengine:       providerGenerator(volcengine.NewDefaultConfig, volcengine.NewDNSProviderConfig),
	Providervscale:           providerGenerator(vscale.NewDefaultConfig, vscale.NewDNSProviderConfig),
	Providervultr:            providerGenerator(vultr.NewDefaultConfig, vultr.NewDNSProviderConfig),
	Providerwebnames:         providerGenerator(webnames.NewDefaultConfig, webnames.NewDNSProviderConfig),
	Providerwebsupport:       providerGenerator(websupport.NewDefaultConfig, websupport.NewDNSProviderConfig),
	Providerwedos:            providerGenerator(wedos.NewDefaultConfig, wedos.NewDNSProviderConfig),
	Providerwestcn:           providerGenerator(westcn.NewDefaultConfig, westcn.NewDNSProviderConfig),
	Provideryandex:           providerGenerator(yandex.NewDefaultConfig, yandex.NewDNSProviderConfig),
	Provideryandex360:        providerGenerator(yandex360.NewDefaultConfig, yandex360.NewDNSProviderConfig),
	Providerzoneee:           providerGenerator(zoneee.NewDefaultConfig, zoneee.NewDNSProviderConfig),
	Providerzonomi:           providerGenerator(zonomi.NewDefaultConfig, zonomi.NewDNSProviderConfig),
}

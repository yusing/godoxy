//go:generate /usr/bin/python3 gen.py

package dnsproviders

import (
	"github.com/go-acme/lego/v4/providers/dns/acmedns"
	"github.com/go-acme/lego/v4/providers/dns/active24"
	"github.com/go-acme/lego/v4/providers/dns/allinkl"
	"github.com/go-acme/lego/v4/providers/dns/arvancloud"
	"github.com/go-acme/lego/v4/providers/dns/auroradns"
	"github.com/go-acme/lego/v4/providers/dns/autodns"
	"github.com/go-acme/lego/v4/providers/dns/axelname"
	"github.com/go-acme/lego/v4/providers/dns/azion"
	"github.com/go-acme/lego/v4/providers/dns/azuredns"
	"github.com/go-acme/lego/v4/providers/dns/bindman"
	"github.com/go-acme/lego/v4/providers/dns/bluecat"
	"github.com/go-acme/lego/v4/providers/dns/bookmyname"
	"github.com/go-acme/lego/v4/providers/dns/bunny"
	"github.com/go-acme/lego/v4/providers/dns/checkdomain"
	"github.com/go-acme/lego/v4/providers/dns/civo"
	"github.com/go-acme/lego/v4/providers/dns/clouddns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/cloudns"
	"github.com/go-acme/lego/v4/providers/dns/cloudru"
	"github.com/go-acme/lego/v4/providers/dns/conoha"
	"github.com/go-acme/lego/v4/providers/dns/conohav3"
	"github.com/go-acme/lego/v4/providers/dns/constellix"
	"github.com/go-acme/lego/v4/providers/dns/corenetworks"
	"github.com/go-acme/lego/v4/providers/dns/cpanel"
	"github.com/go-acme/lego/v4/providers/dns/derak"
	"github.com/go-acme/lego/v4/providers/dns/desec"
	"github.com/go-acme/lego/v4/providers/dns/designate"
	"github.com/go-acme/lego/v4/providers/dns/digitalocean"
	"github.com/go-acme/lego/v4/providers/dns/directadmin"
	"github.com/go-acme/lego/v4/providers/dns/dnshomede"
	"github.com/go-acme/lego/v4/providers/dns/dnsimple"
	"github.com/go-acme/lego/v4/providers/dns/dnsmadeeasy"
	"github.com/go-acme/lego/v4/providers/dns/dode"
	"github.com/go-acme/lego/v4/providers/dns/domeneshop"
	"github.com/go-acme/lego/v4/providers/dns/dreamhost"
	"github.com/go-acme/lego/v4/providers/dns/duckdns"
	"github.com/go-acme/lego/v4/providers/dns/dyn"
	"github.com/go-acme/lego/v4/providers/dns/dyndnsfree"
	"github.com/go-acme/lego/v4/providers/dns/dynu"
	"github.com/go-acme/lego/v4/providers/dns/easydns"
	"github.com/go-acme/lego/v4/providers/dns/edgedns"
	"github.com/go-acme/lego/v4/providers/dns/efficientip"
	"github.com/go-acme/lego/v4/providers/dns/epik"
	"github.com/go-acme/lego/v4/providers/dns/exec"
	"github.com/go-acme/lego/v4/providers/dns/exoscale"
	"github.com/go-acme/lego/v4/providers/dns/f5xc"
	"github.com/go-acme/lego/v4/providers/dns/freemyip"
	"github.com/go-acme/lego/v4/providers/dns/gandi"
	"github.com/go-acme/lego/v4/providers/dns/gandiv5"
	"github.com/go-acme/lego/v4/providers/dns/gcloud"
	"github.com/go-acme/lego/v4/providers/dns/gcore"
	"github.com/go-acme/lego/v4/providers/dns/glesys"
	"github.com/go-acme/lego/v4/providers/dns/godaddy"
	"github.com/go-acme/lego/v4/providers/dns/googledomains"
	"github.com/go-acme/lego/v4/providers/dns/hetzner"
	"github.com/go-acme/lego/v4/providers/dns/hostingde"
	"github.com/go-acme/lego/v4/providers/dns/hosttech"
	"github.com/go-acme/lego/v4/providers/dns/httpnet"
	"github.com/go-acme/lego/v4/providers/dns/httpreq"
	"github.com/go-acme/lego/v4/providers/dns/hurricane"
	"github.com/go-acme/lego/v4/providers/dns/hyperone"
	"github.com/go-acme/lego/v4/providers/dns/ibmcloud"
	"github.com/go-acme/lego/v4/providers/dns/iij"
	"github.com/go-acme/lego/v4/providers/dns/iijdpf"
	"github.com/go-acme/lego/v4/providers/dns/infoblox"
	"github.com/go-acme/lego/v4/providers/dns/infomaniak"
	"github.com/go-acme/lego/v4/providers/dns/internetbs"
	"github.com/go-acme/lego/v4/providers/dns/inwx"
	"github.com/go-acme/lego/v4/providers/dns/ionos"
	"github.com/go-acme/lego/v4/providers/dns/ipv64"
	"github.com/go-acme/lego/v4/providers/dns/iwantmyname"
	"github.com/go-acme/lego/v4/providers/dns/joker"
	"github.com/go-acme/lego/v4/providers/dns/liara"
	"github.com/go-acme/lego/v4/providers/dns/lightsail"
	"github.com/go-acme/lego/v4/providers/dns/limacity"
	"github.com/go-acme/lego/v4/providers/dns/linode"
	"github.com/go-acme/lego/v4/providers/dns/liquidweb"
	"github.com/go-acme/lego/v4/providers/dns/loopia"
	"github.com/go-acme/lego/v4/providers/dns/luadns"
	"github.com/go-acme/lego/v4/providers/dns/mailinabox"
	"github.com/go-acme/lego/v4/providers/dns/manageengine"
	"github.com/go-acme/lego/v4/providers/dns/metaname"
	"github.com/go-acme/lego/v4/providers/dns/metaregistrar"
	"github.com/go-acme/lego/v4/providers/dns/mijnhost"
	"github.com/go-acme/lego/v4/providers/dns/mittwald"
	"github.com/go-acme/lego/v4/providers/dns/myaddr"
	"github.com/go-acme/lego/v4/providers/dns/mydnsjp"
	"github.com/go-acme/lego/v4/providers/dns/namecheap"
	"github.com/go-acme/lego/v4/providers/dns/namedotcom"
	"github.com/go-acme/lego/v4/providers/dns/nearlyfreespeech"
	"github.com/go-acme/lego/v4/providers/dns/netcup"
	"github.com/go-acme/lego/v4/providers/dns/netlify"
	"github.com/go-acme/lego/v4/providers/dns/nicmanager"
	"github.com/go-acme/lego/v4/providers/dns/nicru"
	"github.com/go-acme/lego/v4/providers/dns/nifcloud"
	"github.com/go-acme/lego/v4/providers/dns/njalla"
	"github.com/go-acme/lego/v4/providers/dns/nodion"
	"github.com/go-acme/lego/v4/providers/dns/ns1"
	"github.com/go-acme/lego/v4/providers/dns/oraclecloud"
	"github.com/go-acme/lego/v4/providers/dns/otc"
	"github.com/go-acme/lego/v4/providers/dns/ovh"
	"github.com/go-acme/lego/v4/providers/dns/pdns"
	"github.com/go-acme/lego/v4/providers/dns/plesk"
	"github.com/go-acme/lego/v4/providers/dns/porkbun"
	"github.com/go-acme/lego/v4/providers/dns/rackspace"
	"github.com/go-acme/lego/v4/providers/dns/rainyun"
	"github.com/go-acme/lego/v4/providers/dns/rcodezero"
	"github.com/go-acme/lego/v4/providers/dns/regfish"
	"github.com/go-acme/lego/v4/providers/dns/regru"
	"github.com/go-acme/lego/v4/providers/dns/rfc2136"
	"github.com/go-acme/lego/v4/providers/dns/rimuhosting"
	"github.com/go-acme/lego/v4/providers/dns/route53"
	"github.com/go-acme/lego/v4/providers/dns/safedns"
	"github.com/go-acme/lego/v4/providers/dns/sakuracloud"
	"github.com/go-acme/lego/v4/providers/dns/scaleway"
	"github.com/go-acme/lego/v4/providers/dns/selectel"
	"github.com/go-acme/lego/v4/providers/dns/selectelv2"
	"github.com/go-acme/lego/v4/providers/dns/selfhostde"
	"github.com/go-acme/lego/v4/providers/dns/servercow"
	"github.com/go-acme/lego/v4/providers/dns/shellrent"
	"github.com/go-acme/lego/v4/providers/dns/simply"
	"github.com/go-acme/lego/v4/providers/dns/sonic"
	"github.com/go-acme/lego/v4/providers/dns/spaceship"
	"github.com/go-acme/lego/v4/providers/dns/stackpath"
	"github.com/go-acme/lego/v4/providers/dns/technitium"
	"github.com/go-acme/lego/v4/providers/dns/timewebcloud"
	"github.com/go-acme/lego/v4/providers/dns/transip"
	"github.com/go-acme/lego/v4/providers/dns/ultradns"
	"github.com/go-acme/lego/v4/providers/dns/variomedia"
	"github.com/go-acme/lego/v4/providers/dns/vegadns"
	"github.com/go-acme/lego/v4/providers/dns/vercel"
	"github.com/go-acme/lego/v4/providers/dns/versio"
	"github.com/go-acme/lego/v4/providers/dns/vinyldns"
	"github.com/go-acme/lego/v4/providers/dns/vkcloud"
	"github.com/go-acme/lego/v4/providers/dns/volcengine"
	"github.com/go-acme/lego/v4/providers/dns/vscale"
	"github.com/go-acme/lego/v4/providers/dns/vultr"
	"github.com/go-acme/lego/v4/providers/dns/webnames"
	"github.com/go-acme/lego/v4/providers/dns/websupport"
	"github.com/go-acme/lego/v4/providers/dns/wedos"
	"github.com/go-acme/lego/v4/providers/dns/westcn"
	"github.com/go-acme/lego/v4/providers/dns/yandex"
	"github.com/go-acme/lego/v4/providers/dns/yandex360"
	"github.com/go-acme/lego/v4/providers/dns/zoneedit"
	"github.com/go-acme/lego/v4/providers/dns/zoneee"
	"github.com/go-acme/lego/v4/providers/dns/zonomi"
	"github.com/yusing/go-proxy/internal/autocert"
)

const (
	Local  = "local"
	Pseudo = "pseudo"
)

func InitProviders() {
	autocert.Providers[Local] = autocert.DNSProvider(NewDummyDefaultConfig, NewDummyDNSProviderConfig)
	autocert.Providers[Pseudo] = autocert.DNSProvider(NewDummyDefaultConfig, NewDummyDNSProviderConfig)
	autocert.Providers["acmedns"] = autocert.DNSProvider(acmedns.NewDefaultConfig, acmedns.NewDNSProviderConfig)
	autocert.Providers["active24"] = autocert.DNSProvider(active24.NewDefaultConfig, active24.NewDNSProviderConfig)
	autocert.Providers["allinkl"] = autocert.DNSProvider(allinkl.NewDefaultConfig, allinkl.NewDNSProviderConfig)
	autocert.Providers["arvancloud"] = autocert.DNSProvider(arvancloud.NewDefaultConfig, arvancloud.NewDNSProviderConfig)
	autocert.Providers["auroradns"] = autocert.DNSProvider(auroradns.NewDefaultConfig, auroradns.NewDNSProviderConfig)
	autocert.Providers["autodns"] = autocert.DNSProvider(autodns.NewDefaultConfig, autodns.NewDNSProviderConfig)
	autocert.Providers["axelname"] = autocert.DNSProvider(axelname.NewDefaultConfig, axelname.NewDNSProviderConfig)
	autocert.Providers["azion"] = autocert.DNSProvider(azion.NewDefaultConfig, azion.NewDNSProviderConfig)
	autocert.Providers["azuredns"] = autocert.DNSProvider(azuredns.NewDefaultConfig, azuredns.NewDNSProviderConfig)
	autocert.Providers["bindman"] = autocert.DNSProvider(bindman.NewDefaultConfig, bindman.NewDNSProviderConfig)
	autocert.Providers["bluecat"] = autocert.DNSProvider(bluecat.NewDefaultConfig, bluecat.NewDNSProviderConfig)
	autocert.Providers["bookmyname"] = autocert.DNSProvider(bookmyname.NewDefaultConfig, bookmyname.NewDNSProviderConfig)
	autocert.Providers["bunny"] = autocert.DNSProvider(bunny.NewDefaultConfig, bunny.NewDNSProviderConfig)
	autocert.Providers["checkdomain"] = autocert.DNSProvider(checkdomain.NewDefaultConfig, checkdomain.NewDNSProviderConfig)
	autocert.Providers["civo"] = autocert.DNSProvider(civo.NewDefaultConfig, civo.NewDNSProviderConfig)
	autocert.Providers["clouddns"] = autocert.DNSProvider(clouddns.NewDefaultConfig, clouddns.NewDNSProviderConfig)
	autocert.Providers["cloudflare"] = autocert.DNSProvider(cloudflare.NewDefaultConfig, cloudflare.NewDNSProviderConfig)
	autocert.Providers["cloudns"] = autocert.DNSProvider(cloudns.NewDefaultConfig, cloudns.NewDNSProviderConfig)
	autocert.Providers["cloudru"] = autocert.DNSProvider(cloudru.NewDefaultConfig, cloudru.NewDNSProviderConfig)
	autocert.Providers["conoha"] = autocert.DNSProvider(conoha.NewDefaultConfig, conoha.NewDNSProviderConfig)
	autocert.Providers["conohav3"] = autocert.DNSProvider(conohav3.NewDefaultConfig, conohav3.NewDNSProviderConfig)
	autocert.Providers["constellix"] = autocert.DNSProvider(constellix.NewDefaultConfig, constellix.NewDNSProviderConfig)
	autocert.Providers["corenetworks"] = autocert.DNSProvider(corenetworks.NewDefaultConfig, corenetworks.NewDNSProviderConfig)
	autocert.Providers["cpanel"] = autocert.DNSProvider(cpanel.NewDefaultConfig, cpanel.NewDNSProviderConfig)
	autocert.Providers["derak"] = autocert.DNSProvider(derak.NewDefaultConfig, derak.NewDNSProviderConfig)
	autocert.Providers["desec"] = autocert.DNSProvider(desec.NewDefaultConfig, desec.NewDNSProviderConfig)
	autocert.Providers["designate"] = autocert.DNSProvider(designate.NewDefaultConfig, designate.NewDNSProviderConfig)
	autocert.Providers["digitalocean"] = autocert.DNSProvider(digitalocean.NewDefaultConfig, digitalocean.NewDNSProviderConfig)
	autocert.Providers["directadmin"] = autocert.DNSProvider(directadmin.NewDefaultConfig, directadmin.NewDNSProviderConfig)
	autocert.Providers["dnshomede"] = autocert.DNSProvider(dnshomede.NewDefaultConfig, dnshomede.NewDNSProviderConfig)
	autocert.Providers["dnsimple"] = autocert.DNSProvider(dnsimple.NewDefaultConfig, dnsimple.NewDNSProviderConfig)
	autocert.Providers["dnsmadeeasy"] = autocert.DNSProvider(dnsmadeeasy.NewDefaultConfig, dnsmadeeasy.NewDNSProviderConfig)
	autocert.Providers["dode"] = autocert.DNSProvider(dode.NewDefaultConfig, dode.NewDNSProviderConfig)
	autocert.Providers["domeneshop"] = autocert.DNSProvider(domeneshop.NewDefaultConfig, domeneshop.NewDNSProviderConfig)
	autocert.Providers["dreamhost"] = autocert.DNSProvider(dreamhost.NewDefaultConfig, dreamhost.NewDNSProviderConfig)
	autocert.Providers["duckdns"] = autocert.DNSProvider(duckdns.NewDefaultConfig, duckdns.NewDNSProviderConfig)
	autocert.Providers["dyn"] = autocert.DNSProvider(dyn.NewDefaultConfig, dyn.NewDNSProviderConfig)
	autocert.Providers["dyndnsfree"] = autocert.DNSProvider(dyndnsfree.NewDefaultConfig, dyndnsfree.NewDNSProviderConfig)
	autocert.Providers["dynu"] = autocert.DNSProvider(dynu.NewDefaultConfig, dynu.NewDNSProviderConfig)
	autocert.Providers["easydns"] = autocert.DNSProvider(easydns.NewDefaultConfig, easydns.NewDNSProviderConfig)
	autocert.Providers["edgedns"] = autocert.DNSProvider(edgedns.NewDefaultConfig, edgedns.NewDNSProviderConfig)
	autocert.Providers["efficientip"] = autocert.DNSProvider(efficientip.NewDefaultConfig, efficientip.NewDNSProviderConfig)
	autocert.Providers["epik"] = autocert.DNSProvider(epik.NewDefaultConfig, epik.NewDNSProviderConfig)
	autocert.Providers["exec"] = autocert.DNSProvider(exec.NewDefaultConfig, exec.NewDNSProviderConfig)
	autocert.Providers["exoscale"] = autocert.DNSProvider(exoscale.NewDefaultConfig, exoscale.NewDNSProviderConfig)
	autocert.Providers["f5xc"] = autocert.DNSProvider(f5xc.NewDefaultConfig, f5xc.NewDNSProviderConfig)
	autocert.Providers["freemyip"] = autocert.DNSProvider(freemyip.NewDefaultConfig, freemyip.NewDNSProviderConfig)
	autocert.Providers["gandi"] = autocert.DNSProvider(gandi.NewDefaultConfig, gandi.NewDNSProviderConfig)
	autocert.Providers["gandiv5"] = autocert.DNSProvider(gandiv5.NewDefaultConfig, gandiv5.NewDNSProviderConfig)
	autocert.Providers["gcloud"] = autocert.DNSProvider(gcloud.NewDefaultConfig, gcloud.NewDNSProviderConfig)
	autocert.Providers["gcore"] = autocert.DNSProvider(gcore.NewDefaultConfig, gcore.NewDNSProviderConfig)
	autocert.Providers["glesys"] = autocert.DNSProvider(glesys.NewDefaultConfig, glesys.NewDNSProviderConfig)
	autocert.Providers["godaddy"] = autocert.DNSProvider(godaddy.NewDefaultConfig, godaddy.NewDNSProviderConfig)
	autocert.Providers["googledomains"] = autocert.DNSProvider(googledomains.NewDefaultConfig, googledomains.NewDNSProviderConfig)
	autocert.Providers["hetzner"] = autocert.DNSProvider(hetzner.NewDefaultConfig, hetzner.NewDNSProviderConfig)
	autocert.Providers["hostingde"] = autocert.DNSProvider(hostingde.NewDefaultConfig, hostingde.NewDNSProviderConfig)
	autocert.Providers["hosttech"] = autocert.DNSProvider(hosttech.NewDefaultConfig, hosttech.NewDNSProviderConfig)
	autocert.Providers["httpnet"] = autocert.DNSProvider(httpnet.NewDefaultConfig, httpnet.NewDNSProviderConfig)
	autocert.Providers["httpreq"] = autocert.DNSProvider(httpreq.NewDefaultConfig, httpreq.NewDNSProviderConfig)
	autocert.Providers["hurricane"] = autocert.DNSProvider(hurricane.NewDefaultConfig, hurricane.NewDNSProviderConfig)
	autocert.Providers["hyperone"] = autocert.DNSProvider(hyperone.NewDefaultConfig, hyperone.NewDNSProviderConfig)
	autocert.Providers["ibmcloud"] = autocert.DNSProvider(ibmcloud.NewDefaultConfig, ibmcloud.NewDNSProviderConfig)
	autocert.Providers["iij"] = autocert.DNSProvider(iij.NewDefaultConfig, iij.NewDNSProviderConfig)
	autocert.Providers["iijdpf"] = autocert.DNSProvider(iijdpf.NewDefaultConfig, iijdpf.NewDNSProviderConfig)
	autocert.Providers["infoblox"] = autocert.DNSProvider(infoblox.NewDefaultConfig, infoblox.NewDNSProviderConfig)
	autocert.Providers["infomaniak"] = autocert.DNSProvider(infomaniak.NewDefaultConfig, infomaniak.NewDNSProviderConfig)
	autocert.Providers["internetbs"] = autocert.DNSProvider(internetbs.NewDefaultConfig, internetbs.NewDNSProviderConfig)
	autocert.Providers["inwx"] = autocert.DNSProvider(inwx.NewDefaultConfig, inwx.NewDNSProviderConfig)
	autocert.Providers["ionos"] = autocert.DNSProvider(ionos.NewDefaultConfig, ionos.NewDNSProviderConfig)
	autocert.Providers["ipv64"] = autocert.DNSProvider(ipv64.NewDefaultConfig, ipv64.NewDNSProviderConfig)
	autocert.Providers["iwantmyname"] = autocert.DNSProvider(iwantmyname.NewDefaultConfig, iwantmyname.NewDNSProviderConfig)
	autocert.Providers["joker"] = autocert.DNSProvider(joker.NewDefaultConfig, joker.NewDNSProviderConfig)
	autocert.Providers["liara"] = autocert.DNSProvider(liara.NewDefaultConfig, liara.NewDNSProviderConfig)
	autocert.Providers["lightsail"] = autocert.DNSProvider(lightsail.NewDefaultConfig, lightsail.NewDNSProviderConfig)
	autocert.Providers["limacity"] = autocert.DNSProvider(limacity.NewDefaultConfig, limacity.NewDNSProviderConfig)
	autocert.Providers["linode"] = autocert.DNSProvider(linode.NewDefaultConfig, linode.NewDNSProviderConfig)
	autocert.Providers["liquidweb"] = autocert.DNSProvider(liquidweb.NewDefaultConfig, liquidweb.NewDNSProviderConfig)
	autocert.Providers["loopia"] = autocert.DNSProvider(loopia.NewDefaultConfig, loopia.NewDNSProviderConfig)
	autocert.Providers["luadns"] = autocert.DNSProvider(luadns.NewDefaultConfig, luadns.NewDNSProviderConfig)
	autocert.Providers["mailinabox"] = autocert.DNSProvider(mailinabox.NewDefaultConfig, mailinabox.NewDNSProviderConfig)
	autocert.Providers["manageengine"] = autocert.DNSProvider(manageengine.NewDefaultConfig, manageengine.NewDNSProviderConfig)
	autocert.Providers["metaname"] = autocert.DNSProvider(metaname.NewDefaultConfig, metaname.NewDNSProviderConfig)
	autocert.Providers["metaregistrar"] = autocert.DNSProvider(metaregistrar.NewDefaultConfig, metaregistrar.NewDNSProviderConfig)
	autocert.Providers["mijnhost"] = autocert.DNSProvider(mijnhost.NewDefaultConfig, mijnhost.NewDNSProviderConfig)
	autocert.Providers["mittwald"] = autocert.DNSProvider(mittwald.NewDefaultConfig, mittwald.NewDNSProviderConfig)
	autocert.Providers["myaddr"] = autocert.DNSProvider(myaddr.NewDefaultConfig, myaddr.NewDNSProviderConfig)
	autocert.Providers["mydnsjp"] = autocert.DNSProvider(mydnsjp.NewDefaultConfig, mydnsjp.NewDNSProviderConfig)
	autocert.Providers["namecheap"] = autocert.DNSProvider(namecheap.NewDefaultConfig, namecheap.NewDNSProviderConfig)
	autocert.Providers["namedotcom"] = autocert.DNSProvider(namedotcom.NewDefaultConfig, namedotcom.NewDNSProviderConfig)
	autocert.Providers["nearlyfreespeech"] = autocert.DNSProvider(nearlyfreespeech.NewDefaultConfig, nearlyfreespeech.NewDNSProviderConfig)
	autocert.Providers["netcup"] = autocert.DNSProvider(netcup.NewDefaultConfig, netcup.NewDNSProviderConfig)
	autocert.Providers["netlify"] = autocert.DNSProvider(netlify.NewDefaultConfig, netlify.NewDNSProviderConfig)
	autocert.Providers["nicmanager"] = autocert.DNSProvider(nicmanager.NewDefaultConfig, nicmanager.NewDNSProviderConfig)
	autocert.Providers["nicru"] = autocert.DNSProvider(nicru.NewDefaultConfig, nicru.NewDNSProviderConfig)
	autocert.Providers["nifcloud"] = autocert.DNSProvider(nifcloud.NewDefaultConfig, nifcloud.NewDNSProviderConfig)
	autocert.Providers["njalla"] = autocert.DNSProvider(njalla.NewDefaultConfig, njalla.NewDNSProviderConfig)
	autocert.Providers["nodion"] = autocert.DNSProvider(nodion.NewDefaultConfig, nodion.NewDNSProviderConfig)
	autocert.Providers["ns1"] = autocert.DNSProvider(ns1.NewDefaultConfig, ns1.NewDNSProviderConfig)
	autocert.Providers["oraclecloud"] = autocert.DNSProvider(oraclecloud.NewDefaultConfig, oraclecloud.NewDNSProviderConfig)
	autocert.Providers["otc"] = autocert.DNSProvider(otc.NewDefaultConfig, otc.NewDNSProviderConfig)
	autocert.Providers["ovh"] = autocert.DNSProvider(ovh.NewDefaultConfig, ovh.NewDNSProviderConfig)
	autocert.Providers["pdns"] = autocert.DNSProvider(pdns.NewDefaultConfig, pdns.NewDNSProviderConfig)
	autocert.Providers["plesk"] = autocert.DNSProvider(plesk.NewDefaultConfig, plesk.NewDNSProviderConfig)
	autocert.Providers["porkbun"] = autocert.DNSProvider(porkbun.NewDefaultConfig, porkbun.NewDNSProviderConfig)
	autocert.Providers["rackspace"] = autocert.DNSProvider(rackspace.NewDefaultConfig, rackspace.NewDNSProviderConfig)
	autocert.Providers["rainyun"] = autocert.DNSProvider(rainyun.NewDefaultConfig, rainyun.NewDNSProviderConfig)
	autocert.Providers["rcodezero"] = autocert.DNSProvider(rcodezero.NewDefaultConfig, rcodezero.NewDNSProviderConfig)
	autocert.Providers["regfish"] = autocert.DNSProvider(regfish.NewDefaultConfig, regfish.NewDNSProviderConfig)
	autocert.Providers["regru"] = autocert.DNSProvider(regru.NewDefaultConfig, regru.NewDNSProviderConfig)
	autocert.Providers["rfc2136"] = autocert.DNSProvider(rfc2136.NewDefaultConfig, rfc2136.NewDNSProviderConfig)
	autocert.Providers["rimuhosting"] = autocert.DNSProvider(rimuhosting.NewDefaultConfig, rimuhosting.NewDNSProviderConfig)
	autocert.Providers["route53"] = autocert.DNSProvider(route53.NewDefaultConfig, route53.NewDNSProviderConfig)
	autocert.Providers["safedns"] = autocert.DNSProvider(safedns.NewDefaultConfig, safedns.NewDNSProviderConfig)
	autocert.Providers["sakuracloud"] = autocert.DNSProvider(sakuracloud.NewDefaultConfig, sakuracloud.NewDNSProviderConfig)
	autocert.Providers["scaleway"] = autocert.DNSProvider(scaleway.NewDefaultConfig, scaleway.NewDNSProviderConfig)
	autocert.Providers["selectel"] = autocert.DNSProvider(selectel.NewDefaultConfig, selectel.NewDNSProviderConfig)
	autocert.Providers["selectelv2"] = autocert.DNSProvider(selectelv2.NewDefaultConfig, selectelv2.NewDNSProviderConfig)
	autocert.Providers["selfhostde"] = autocert.DNSProvider(selfhostde.NewDefaultConfig, selfhostde.NewDNSProviderConfig)
	autocert.Providers["servercow"] = autocert.DNSProvider(servercow.NewDefaultConfig, servercow.NewDNSProviderConfig)
	autocert.Providers["shellrent"] = autocert.DNSProvider(shellrent.NewDefaultConfig, shellrent.NewDNSProviderConfig)
	autocert.Providers["simply"] = autocert.DNSProvider(simply.NewDefaultConfig, simply.NewDNSProviderConfig)
	autocert.Providers["sonic"] = autocert.DNSProvider(sonic.NewDefaultConfig, sonic.NewDNSProviderConfig)
	autocert.Providers["spaceship"] = autocert.DNSProvider(spaceship.NewDefaultConfig, spaceship.NewDNSProviderConfig)
	autocert.Providers["stackpath"] = autocert.DNSProvider(stackpath.NewDefaultConfig, stackpath.NewDNSProviderConfig)
	autocert.Providers["technitium"] = autocert.DNSProvider(technitium.NewDefaultConfig, technitium.NewDNSProviderConfig)
	autocert.Providers["timewebcloud"] = autocert.DNSProvider(timewebcloud.NewDefaultConfig, timewebcloud.NewDNSProviderConfig)
	autocert.Providers["transip"] = autocert.DNSProvider(transip.NewDefaultConfig, transip.NewDNSProviderConfig)
	autocert.Providers["ultradns"] = autocert.DNSProvider(ultradns.NewDefaultConfig, ultradns.NewDNSProviderConfig)
	autocert.Providers["variomedia"] = autocert.DNSProvider(variomedia.NewDefaultConfig, variomedia.NewDNSProviderConfig)
	autocert.Providers["vegadns"] = autocert.DNSProvider(vegadns.NewDefaultConfig, vegadns.NewDNSProviderConfig)
	autocert.Providers["vercel"] = autocert.DNSProvider(vercel.NewDefaultConfig, vercel.NewDNSProviderConfig)
	autocert.Providers["versio"] = autocert.DNSProvider(versio.NewDefaultConfig, versio.NewDNSProviderConfig)
	autocert.Providers["vinyldns"] = autocert.DNSProvider(vinyldns.NewDefaultConfig, vinyldns.NewDNSProviderConfig)
	autocert.Providers["vkcloud"] = autocert.DNSProvider(vkcloud.NewDefaultConfig, vkcloud.NewDNSProviderConfig)
	autocert.Providers["volcengine"] = autocert.DNSProvider(volcengine.NewDefaultConfig, volcengine.NewDNSProviderConfig)
	autocert.Providers["vscale"] = autocert.DNSProvider(vscale.NewDefaultConfig, vscale.NewDNSProviderConfig)
	autocert.Providers["vultr"] = autocert.DNSProvider(vultr.NewDefaultConfig, vultr.NewDNSProviderConfig)
	autocert.Providers["webnames"] = autocert.DNSProvider(webnames.NewDefaultConfig, webnames.NewDNSProviderConfig)
	autocert.Providers["websupport"] = autocert.DNSProvider(websupport.NewDefaultConfig, websupport.NewDNSProviderConfig)
	autocert.Providers["wedos"] = autocert.DNSProvider(wedos.NewDefaultConfig, wedos.NewDNSProviderConfig)
	autocert.Providers["westcn"] = autocert.DNSProvider(westcn.NewDefaultConfig, westcn.NewDNSProviderConfig)
	autocert.Providers["yandex"] = autocert.DNSProvider(yandex.NewDefaultConfig, yandex.NewDNSProviderConfig)
	autocert.Providers["yandex360"] = autocert.DNSProvider(yandex360.NewDefaultConfig, yandex360.NewDNSProviderConfig)
	autocert.Providers["zoneedit"] = autocert.DNSProvider(zoneedit.NewDefaultConfig, zoneedit.NewDNSProviderConfig)
	autocert.Providers["zoneee"] = autocert.DNSProvider(zoneee.NewDefaultConfig, zoneee.NewDNSProviderConfig)
	autocert.Providers["zonomi"] = autocert.DNSProvider(zonomi.NewDefaultConfig, zonomi.NewDNSProviderConfig)
}

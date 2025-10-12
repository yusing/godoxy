import os
import re

import requests


def unquote(s: str) -> str:
    return s.strip().strip('"')


url = "https://raw.githubusercontent.com/go-acme/lego/refs/heads/master/providers/dns/zz_gen_dns_providers.go"
import_prefix = "github.com/go-acme/lego/v4/providers/dns/"
response = requests.get(url)
data: list[str] = [unquote(i) for i in response.text.split("\n") if import_prefix in i]
data_map = {item.split("/")[-1]: item for item in data}

header = "//go:generate /usr/bin/python3 gen.py\n\npackage dnsproviders\n\n"
names: list[str] = [
    'Local = "local"',
    'Pseudo = "pseudo"',
]
imports: list[str] = ['"github.com/yusing/godoxy/internal/autocert"']
genMap: list[str] = [
    "autocert.Providers[Local] = autocert.DNSProvider(NewDummyDefaultConfig, NewDummyDNSProviderConfig)",
    "autocert.Providers[Pseudo] = autocert.DNSProvider(NewDummyDefaultConfig, NewDummyDNSProviderConfig)",
]

allowlist = [
    "acmedns",
    "azuredns",
    "cloudflare",
    "cloudns",
    "clouddns",
    "digitalocean",
    "duckdns",
    "edgedns",
    "gcloud",
    "godaddy",
    "googledomains",
    "hetzner",
    # "hostinger", # TODO: uncomment when v4.27.0 is released
    "httpreq",
    "ionos",
    "linode",
    "namecheap",
    "netcup",
    "netlify",
    "oraclecloud",
    "ovh",
    "porkbun",
    "rfc2136",
    "scaleway",
    "spaceship",
    "vercel",
    "vultr",

    "timewebcloud"
]

for name in allowlist:
    import_str = data_map.get(name)
    if import_str is None:
        continue
    imports.append(f'"{import_str}"')
    genMap.append(
        f'autocert.Providers["{name}"] = autocert.DNSProvider({name}.NewDefaultConfig, {name}.NewDNSProviderConfig)'
    )

with open("providers.go", "w") as f:
    f.write(header)
    f.write("import (\n")
    f.write("\n".join(imports))
    f.write("\n)\n\n")
    f.write("const (\n")
    f.write("\n".join(names))
    f.write("\n)\n\n")
    f.write("func InitProviders() {\n")
    f.write("\n".join(genMap))
    f.write("\n}\n\n")

os.execvp("go", ["go", "fmt", "providers.go"])

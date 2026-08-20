import os
import re

import requests


def unquote(s: str) -> str:
    return s.strip().strip('"')


# Pin to v5.3.1 tag to match go.mod dependency version for immutable source reference
url = "https://raw.githubusercontent.com/go-acme/lego/refs/tags/v5.3.1/providers/dns/zz_gen_dns_providers.go"
import_prefix = "github.com/go-acme/lego/v5/providers/dns/"
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
    "desec",
    "digitalocean",
    "dnsupdate",
    "duckdns",
    "edgedns",
    "gcloud",
    "godaddy",
    "hetzner",
    "hostinger",
    "httpreq",
    "ionos",
    "inwx",
    "linode",
    "namecheap",
    "netcup",
    "netlify",
    "oraclecloud",
    "ovh",
    "porkbun",
    # "route53",
    "scaleway",
    "spaceship",
    "vercel",
    "vultr",
    "timewebcloud",
]

# Extra config keys kept so existing user configs keep working after lego renamed
# the package: package name -> additional provider keys.
aliases = {
    "dnsupdate": ["rfc2136"],
}

missing = [name for name in allowlist if name not in data_map]
if missing:
    raise SystemExit(
        f"allowlisted providers not found upstream: {', '.join(missing)}\n"
        f"they were renamed or removed in lego; update the allowlist ({url})"
    )

for name in allowlist:
    import_str = data_map[name]
    imports.append(f'"{import_str}"')
    for key in [name, *aliases.get(name, [])]:
        genMap.append(
            f'autocert.Providers["{key}"] = autocert.DNSProvider({name}.NewDefaultConfig, {name}.NewDNSProviderConfig)'
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

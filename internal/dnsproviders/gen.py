import requests
import os

class Entry:
  def __init__(self, name: str, type: str, **kwargs) -> None:
    self.name = name
    self.type = type
  
url = "https://api.github.com/repos/go-acme/lego/contents/providers/dns"
response = requests.get(url)
data: list[Entry] = [Entry(**i) for i in response.json()]

header = "//go:generate /usr/bin/python3 gen.py\n\npackage dnsproviders\n\n"
names: list[str] = [
  "Local = \"local\"",
  "Pseudo = \"pseudo\"",
]
imports: list[str] = [
  "\"github.com/yusing/go-proxy/internal/autocert\""
]
genMap: list[str] = [
  "autocert.Providers[Local] = autocert.DNSProvider(NewDummyDefaultConfig, NewDummyDNSProviderConfig)",
  "autocert.Providers[Pseudo] = autocert.DNSProvider(NewDummyDefaultConfig, NewDummyDNSProviderConfig)",
]

blacklists = [
  "internal",
  # deprecated 
  "azure",
  "brandit",
  "cloudxns",
  "dnspod",
  "mythicbeasts", 
  "yandexcloud",
  # dependencies issue
  "namesilo",
  "binarylane",
  "edgeone",
  # has some annoying dependencies
  "baiducloud",
  "huaweicloud",
  "tencentcloud",
  "alidns"
]

for item in data:
    if item.type != "dir" or item.name in blacklists:
      continue
    imports.append(f"\"github.com/go-acme/lego/v4/providers/dns/{item.name}\"")
    genMap.append(f"autocert.Providers[\"{item.name}\"] = autocert.DNSProvider({item.name}.NewDefaultConfig, {item.name}.NewDNSProviderConfig)")
    
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
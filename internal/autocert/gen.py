import requests
import os

class Entry:
  def __init__(self, name: str, type: str, **kwargs) -> None:
    self.name = name
    self.type = type
  
url = "https://api.github.com/repos/go-acme/lego/contents/providers/dns"
response = requests.get(url)
data: list[Entry] = [Entry(**i) for i in response.json()]

header = "//go:generate /usr/bin/python3 gen.py\n\npackage autocert\n\n"
names: list[str] = [
  "ProviderLocal = \"local\"",
  "ProviderPseudo = \"pseudo\"",
]
imports: list[str] = []
genMap: list[str] = [
  "ProviderLocal: providerGenerator(NewDummyDefaultConfig, NewDummyDNSProviderConfig),",
  "ProviderPseudo: providerGenerator(NewDummyDefaultConfig, NewDummyDNSProviderConfig),",
]

blacklists = [
  "internal",
  # deprecated 
  "azure",
  "brandit",
  "cloudxns",
  "dnspod",
  "mythicbeasts", 
  "yandexcloud"
]

for item in data:
    if item.type != "dir" or item.name in blacklists:
      continue
    imports.append(f"import \"github.com/go-acme/lego/v4/providers/dns/{item.name}\"")
    names.append(f"Provider{item.name} = \"{item.name}\"")
    genMap.append(f"Provider{item.name}: providerGenerator({item.name}.NewDefaultConfig, {item.name}.NewDNSProviderConfig),")
    
with open("providers.go", "w") as f:
  f.write(header)
  f.write("\n".join(imports))
  f.write("\n\n")
  f.write("const (\n")
  f.write("\n".join(names))
  f.write("\n)\n\n")
  f.write("var providers = map[string]ProviderGenerator{\n")
  f.write("\n".join(genMap))
  f.write("\n}\n\n")
  
os.execvp("go", ["go", "fmt", "providers.go"])
module github.com/yusing/go-proxy

go 1.24.2

require (
	github.com/PuerkitoBio/goquery v1.10.3 // parsing HTML for extract fav icon
	github.com/coder/websocket v1.8.13 // websocket for API and agent
	github.com/coreos/go-oidc/v3 v3.14.1 // oidc authentication
	github.com/docker/docker v28.1.1+incompatible // docker daemon
	github.com/fsnotify/fsnotify v1.9.0 // file watcher
	github.com/go-acme/lego/v4 v4.23.1 // acme client
	github.com/go-playground/validator/v10 v10.26.0 // validator
	github.com/gobwas/glob v0.2.3 // glob matcher for route rules
	github.com/golang-jwt/jwt/v5 v5.2.2 // jwt for default auth
	github.com/gotify/server/v2 v2.6.1 // reference the Message struct for json response
	github.com/lithammer/fuzzysearch v1.1.8 // fuzzy search for searching icons and filtering metrics
	github.com/prometheus/client_golang v1.22.0 // metrics
	github.com/puzpuzpuz/xsync/v3 v3.5.1 // lock free map for concurrent operations
	github.com/rs/zerolog v1.34.0 // logging
	github.com/shirou/gopsutil/v4 v4.25.3 // system info metrics
	github.com/vincent-petithory/dataurl v1.0.0 // data url for fav icon
	golang.org/x/crypto v0.37.0 // encrypting password with bcrypt
	golang.org/x/net v0.39.0 // HTTP header utilities
	golang.org/x/oauth2 v0.29.0 // oauth2 authentication
	golang.org/x/text v0.24.0 // string utilities
	golang.org/x/time v0.11.0 // time utilities
	gopkg.in/yaml.v3 v3.0.1 // yaml parsing for different config files
)

replace github.com/coreos/go-oidc/v3 => github.com/godoxy-app/go-oidc/v3 v3.14.2

require (
	github.com/bytedance/sonic v1.13.2
	github.com/docker/cli v28.1.1+incompatible
	github.com/luthermonson/go-proxmox v0.2.2
	github.com/spf13/afero v1.14.0
	github.com/stretchr/testify v1.10.0
	go.uber.org/atomic v1.11.0
)

replace github.com/docker/docker => github.com/godoxy-app/docker v0.0.0-20250418000134-7af8fd7b079e

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/goterm v1.0.4 // indirect
	github.com/bytedance/sonic/loader v0.2.4 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/cloudflare-go v0.115.0 // indirect
	github.com/cloudwego/base64x v0.1.5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/diskfs/go-diskfs v1.6.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ebitengine/purego v0.8.2 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-jose/go-jose/v4 v4.1.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.10 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20250317134145-8bc96cf8fc35 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.65 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nrdcg/porkbun v0.4.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/ovh/go-ovh v1.7.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.63.0 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/sirupsen/logrus v1.9.4-0.20230606125235-dd1b4c2e81af // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/otel v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	golang.org/x/arch v0.16.0 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/tools v0.32.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
)

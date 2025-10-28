module github.com/yusing/godoxy

go 1.25.3

replace github.com/yusing/godoxy/agent => ./agent

replace github.com/yusing/godoxy/internal/dnsproviders => ./internal/dnsproviders

replace github.com/coreos/go-oidc/v3 => ./internal/go-oidc

replace github.com/shirou/gopsutil/v4 => ./internal/gopsutil

replace github.com/yusing/goutils => ./goutils

require (
	github.com/PuerkitoBio/goquery v1.10.3 // parsing HTML for extract fav icon
	github.com/coreos/go-oidc/v3 v3.16.0 // oidc authentication
	github.com/docker/docker v28.5.1+incompatible // docker daemon
	github.com/fsnotify/fsnotify v1.9.0 // file watcher
	github.com/gin-gonic/gin v1.11.0 // api server
	github.com/go-acme/lego/v4 v4.27.0 // acme client
	github.com/go-playground/validator/v10 v10.28.0 // validator
	github.com/gobwas/glob v0.2.3 // glob matcher for route rules
	github.com/gorilla/websocket v1.5.3 // websocket for API and agent
	github.com/gotify/server/v2 v2.7.3 // reference the Message struct for json response
	github.com/lithammer/fuzzysearch v1.1.8 // fuzzy search for searching icons and filtering metrics
	github.com/pires/go-proxyproto v0.8.1 // proxy protocol support
	github.com/puzpuzpuz/xsync/v4 v4.2.0 // lock free map for concurrent operations
	github.com/rs/zerolog v1.34.0 // logging
	github.com/vincent-petithory/dataurl v1.0.0 // data url for fav icon
	golang.org/x/crypto v0.43.0 // encrypting password with bcrypt
	golang.org/x/net v0.46.0 // HTTP header utilities
	golang.org/x/oauth2 v0.32.0 // oauth2 authentication
	golang.org/x/sync v0.17.0
	golang.org/x/time v0.14.0 // time utilities
)

require (
	github.com/docker/cli v28.5.1+incompatible
	github.com/goccy/go-yaml v1.18.0 // yaml parsing for different config files
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/luthermonson/go-proxmox v0.2.3
	github.com/oschwald/maxminddb-golang v1.13.1
	github.com/quic-go/quic-go v0.55.0 // indirect; http3 support
	github.com/samber/slog-zerolog/v2 v2.8.0 // indirect
	github.com/spf13/afero v1.15.0
	github.com/stretchr/testify v1.11.1
	github.com/yusing/ds v0.3.1
	github.com/yusing/godoxy/agent v0.0.0-20251028124446-1797a222cd18
	github.com/yusing/godoxy/internal/dnsproviders v0.0.0-20251028124446-1797a222cd18
	github.com/yusing/goutils v0.7.0
)

require (
	cloud.google.com/go/auth v0.17.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.19.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns v1.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph v0.9.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.5.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/buger/goterm v1.0.4 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/diskfs/go-diskfs v1.7.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/docker/go-connections v0.6.0
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ebitengine/purego v0.9.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/gabriel-vasile/mimetype v1.4.10 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/googleapis/gax-go/v2 v2.15.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/json-iterator/go v1.1.13-0.20220915233716-71ac16282d12 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.68 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/nrdcg/goacmedns v0.2.0 // indirect
	github.com/nrdcg/porkbun v0.4.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/ovh/go-ovh v1.9.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/samber/lo v1.52.0 // indirect
	github.com/samber/slog-common v0.19.0 // indirect
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.35 // indirect
	github.com/sirupsen/logrus v1.9.4-0.20230606125235-dd1b4c2e81af // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.63.0
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.uber.org/atomic v1.11.0
	go.uber.org/ratelimit v0.3.1 // indirect
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	golang.org/x/tools v0.38.0 // indirect
	google.golang.org/api v0.253.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc v1.76.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require (
	github.com/bytedance/sonic v1.14.2
	github.com/shirou/gopsutil/v4 v4.25.9
	github.com/valyala/fasthttp v1.68.0
	github.com/yusing/gointernals v0.1.16
)

require (
	github.com/akamai/AkamaiOPEN-edgegrid-golang/v11 v11.1.0 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/bytedance/gopkg v0.1.3 // indirect
	github.com/bytedance/sonic/loader v0.4.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cloudwego/base64x v0.1.6 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/gin-contrib/sse v1.1.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-ozzo/ozzo-validation/v4 v4.3.0 // indirect
	github.com/go-resty/resty/v2 v2.16.5 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/klauspost/compress v1.18.1 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/linode/linodego v1.60.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/moby/sys/atomicwriter v0.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/nrdcg/oci-go-sdk/common/v1065 v1065.102.1 // indirect
	github.com/nrdcg/oci-go-sdk/dns/v1065 v1065.102.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ugorji/go/codec v1.3.1 // indirect
	github.com/ulikunitz/xz v0.5.14 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/vultr/govultr/v3 v3.24.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.37.0 // indirect
	golang.org/x/arch v0.22.0 // indirect
	google.golang.org/genproto v0.0.0-20250908214217-97024824d090 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250826171959-ef028d996bc1 // indirect
)

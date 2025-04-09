module github.com/yusing/go-proxy

go 1.24.2

// misc

require (
	github.com/fsnotify/fsnotify v1.9.0 // file watcher
	github.com/go-acme/lego/v4 v4.22.2 // acme client
	github.com/go-playground/validator/v10 v10.26.0 // validator
	github.com/gobwas/glob v0.2.3 // glob matcher for route rules
	github.com/gotify/server/v2 v2.6.1 // reference the Message struct for json response
	github.com/lithammer/fuzzysearch v1.1.8 // fuzzy search for searching icons and filtering metrics
	github.com/puzpuzpuz/xsync/v3 v3.5.1 // lock free map for concurrent operations
	golang.org/x/text v0.24.0 // string utilities
	golang.org/x/time v0.11.0 // time utilities
	gopkg.in/yaml.v3 v3.0.1 // yaml parsing for different config files
)

// http

require (
	github.com/coder/websocket v1.8.13 // websocket for API and agent
	github.com/quic-go/quic-go v0.50.1 // http3 server
	golang.org/x/net v0.39.0 // HTTP header utilities
)

// authentication

require (
	github.com/coreos/go-oidc/v3 v3.14.1 // oidc authentication
	github.com/golang-jwt/jwt/v5 v5.2.2 // jwt for default auth
	golang.org/x/crypto v0.37.0 // encrypting password with bcrypt
	golang.org/x/oauth2 v0.29.0 // oauth2 authentication
)

// favicon extraction
require (
	github.com/PuerkitoBio/goquery v1.10.2 // parsing HTML for extract fav icon
	github.com/vincent-petithory/dataurl v1.0.0 // data url for fav icon
)

// docker

require (
	github.com/docker/cli v28.0.4+incompatible // docker cli
	github.com/docker/docker v28.0.4+incompatible // docker daemon
	github.com/docker/go-connections v0.5.0 // docker connection utilities
)

// logging

require (
	github.com/rs/zerolog v1.34.0 // logging
	github.com/samber/slog-zerolog/v2 v2.7.3 // zerlog to slog adapter for quic-go
)

// metrics

require (
	github.com/prometheus/client_golang v1.22.0 // metrics
	github.com/shirou/gopsutil/v4 v4.25.3 // system info metrics
	github.com/stretchr/testify v1.10.0 // testing utilities
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/buger/goterm v1.0.4 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/cloudflare-go v0.115.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/diskfs/go-diskfs v1.5.2 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ebitengine/purego v0.8.2 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/gabriel-vasile/mimetype v1.4.8 // indirect
	github.com/go-jose/go-jose/v4 v4.1.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/pprof v0.0.0-20250403155104-27863c87afa6 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20250317134145-8bc96cf8fc35 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.65 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nrdcg/porkbun v0.4.0 // indirect
	github.com/onsi/ginkgo/v2 v2.23.4 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/ovh/go-ovh v1.7.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.63.0 // indirect
	github.com/prometheus/procfs v0.16.0 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/samber/lo v1.49.1 // indirect
	github.com/samber/slog-common v0.18.1 // indirect
	github.com/sirupsen/logrus v1.9.4-0.20230606125235-dd1b4c2e81af // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.60.0 // indirect
	go.opentelemetry.io/otel v1.35.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.30.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.uber.org/mock v0.5.1 // indirect
	golang.org/x/exp v0.0.0-20250408133849-7e4ce0ab07d0 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/tools v0.32.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gotest.tools/v3 v3.5.1 // indirect
)

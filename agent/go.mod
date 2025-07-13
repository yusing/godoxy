module github.com/yusing/go-proxy/agent

go 1.24.5

replace github.com/yusing/go-proxy => ..

replace github.com/yusing/go-proxy/socketproxy => ../socket-proxy

replace github.com/yusing/go-proxy/internal/utils => ../internal/utils

replace github.com/docker/docker => github.com/godoxy-app/docker v0.0.0-20250523125835-a2474a6ebe30

replace github.com/shirou/gopsutil/v4 => github.com/godoxy-app/gopsutil/v4 v4.0.0-20250607110153-34d627ba1b5d

require (
	github.com/gorilla/websocket v1.5.3
	github.com/pion/dtls/v3 v3.0.6
	github.com/puzpuzpuz/xsync/v4 v4.1.0
	github.com/rs/zerolog v1.34.0
	github.com/stretchr/testify v1.10.0
	github.com/yusing/go-proxy v0.15.1
	github.com/yusing/go-proxy/internal/utils v0.0.0
	github.com/yusing/go-proxy/socketproxy v0.0.0-00010101000000-000000000000
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/PuerkitoBio/goquery v1.10.3 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/cli v28.3.0+incompatible // indirect
	github.com/docker/docker v28.3.0+incompatible // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.26.0 // indirect
	github.com/goccy/go-yaml v1.18.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/gotify/server/v2 v2.6.3 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/lithammer/fuzzysearch v1.1.8 // indirect
	github.com/lufia/plan9stats v0.0.0-20250317134145-8bc96cf8fc35 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/oschwald/maxminddb-golang v1.13.1 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/transport/v3 v3.0.7 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/quic-go/quic-go v0.53.0 // indirect
	github.com/samber/lo v1.51.0 // indirect
	github.com/samber/slog-common v0.19.0 // indirect
	github.com/samber/slog-zerolog/v2 v2.7.3 // indirect
	github.com/shirou/gopsutil/v4 v4.25.5 // indirect
	github.com/sirupsen/logrus v1.9.4-0.20230606125235-dd1b4c2e81af // indirect
	github.com/spf13/afero v1.14.0 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/vincent-petithory/dataurl v1.0.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/mock v0.5.2 // indirect
	golang.org/x/crypto v0.39.0 // indirect
	golang.org/x/mod v0.25.0 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sync v0.15.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	golang.org/x/tools v0.34.0 // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

module github.com/yusing/go-proxy

go 1.24.5

replace github.com/yusing/go-proxy/agent => ./agent

replace github.com/yusing/go-proxy/internal/dnsproviders => ./internal/dnsproviders

replace github.com/yusing/go-proxy/internal/utils => ./internal/utils

replace github.com/coreos/go-oidc/v3 => github.com/godoxy-app/go-oidc/v3 v3.0.0-20250523122447-f078841dec22

replace github.com/docker/docker => github.com/godoxy-app/docker v0.0.0-20250523125835-a2474a6ebe30

replace github.com/shirou/gopsutil/v4 => github.com/godoxy-app/gopsutil/v4 v4.0.0-20250607110153-34d627ba1b5d

require (
	github.com/PuerkitoBio/goquery v1.10.3 // parsing HTML for extract fav icon
	github.com/coreos/go-oidc/v3 v3.14.1 // oidc authentication
	github.com/docker/docker v28.3.0+incompatible // docker daemon
	github.com/fsnotify/fsnotify v1.9.0 // file watcher
	github.com/go-acme/lego/v4 v4.23.1 // acme client
	github.com/go-playground/validator/v10 v10.26.0 // validator
	github.com/gobwas/glob v0.2.3 // glob matcher for route rules
	github.com/gorilla/websocket v1.5.3 // websocket for API and agent
	github.com/gotify/server/v2 v2.6.3 // reference the Message struct for json response
	github.com/lithammer/fuzzysearch v1.1.8 // fuzzy search for searching icons and filtering metrics
	github.com/puzpuzpuz/xsync/v4 v4.1.0 // lock free map for concurrent operations
	github.com/rs/zerolog v1.34.0 // logging
	github.com/shirou/gopsutil/v4 v4.25.5 // system info metrics
	github.com/vincent-petithory/dataurl v1.0.0 // data url for fav icon
	golang.org/x/crypto v0.39.0 // encrypting password with bcrypt
	golang.org/x/net v0.41.0 // HTTP header utilities
	golang.org/x/oauth2 v0.30.0 // oauth2 authentication
	golang.org/x/sync v0.15.0
	golang.org/x/time v0.12.0 // time utilities
)

require (
	github.com/docker/cli v28.3.0+incompatible
	github.com/goccy/go-yaml v1.18.0 // yaml parsing for different config files
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/luthermonson/go-proxmox v0.2.2
	github.com/oschwald/maxminddb-golang v1.13.1
	github.com/quic-go/quic-go v0.53.0
	github.com/samber/slog-zerolog/v2 v2.7.3
	github.com/spf13/afero v1.14.0
	github.com/stretchr/testify v1.10.0
	github.com/yusing/go-proxy/agent v0.0.0-20250628064654-5631b1540abc
	github.com/yusing/go-proxy/internal/dnsproviders v0.0.0-20250628064654-5631b1540abc
	github.com/yusing/go-proxy/internal/utils v0.0.0
)

require (
	cloud.google.com/go/auth v0.16.2 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.7.0 // indirect
	github.com/AdamSLevy/jsonrpc2/v14 v14.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.18.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.10.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dns/armdns v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns v1.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph v0.9.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.4.2 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/OpenDNS/vegadns2client v0.0.0-20180418235048-a3fa4a771d87 // indirect
	github.com/akamai/AkamaiOPEN-edgegrid-golang v1.2.2 // indirect
	github.com/aliyun/alibaba-cloud-sdk-go v1.63.107 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/aws/aws-sdk-go-v2 v1.36.5 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.29.17 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.70 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/lightsail v1.43.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/route53 v1.52.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.34.0 // indirect
	github.com/aws/smithy-go v1.22.4 // indirect
	github.com/baidubce/bce-sdk-go v0.9.233 // indirect
	github.com/benbjohnson/clock v1.3.5 // indirect
	github.com/boombuler/barcode v1.0.2 // indirect
	github.com/buger/goterm v1.0.4 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/civo/civogo v0.6.1 // indirect
	github.com/cloudflare/cloudflare-go v0.115.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/diskfs/go-diskfs v1.6.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/djherbis/times v1.6.0 // indirect
	github.com/dnsimple/dnsimple-go v1.7.0 // indirect
	github.com/docker/go-connections v0.5.0
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/exoscale/egoscale/v3 v3.1.21 // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fxamacker/cbor/v2 v2.8.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.9 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-jose/go-jose/v4 v4.1.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-resty/resty/v2 v2.16.5 // indirect
	github.com/go-viper/mapstructure/v2 v2.3.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect; indirectindirect
	github.com/gofrs/flock v0.12.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/googleapis/gax-go/v2 v2.14.2 // indirect
	github.com/gophercloud/gophercloud v1.14.1 // indirect
	github.com/gophercloud/utils v0.0.0-20231010081019-80377eca5d56 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/huaweicloud/huaweicloud-sdk-go-v3 v0.1.156 // indirect
	github.com/iij/doapi v0.0.0-20190504054126-0bbf12d6d7df // indirect
	github.com/infobloxopen/infoblox-go-client/v2 v2.10.0 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/k0kubun/go-ansi v0.0.0-20180517002512-3bf9e2903213 // indirect
	github.com/kolo/xmlrpc v0.0.0-20220921171641-a4b6fa1dd06b // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/labbsr0x/bindman-dns-webhook v1.0.2 // indirect
	github.com/labbsr0x/goh v1.0.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/linode/linodego v1.52.1 // indirect
	github.com/liquidweb/liquidweb-cli v0.7.0 // indirect
	github.com/liquidweb/liquidweb-go v1.6.4 // indirect
	github.com/lufia/plan9stats v0.0.0-20250317134145-8bc96cf8fc35 // indirect
	github.com/magefile/mage v1.15.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.66 // indirect
	github.com/mimuret/golang-iij-dpf v0.9.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/namedotcom/go v0.0.0-20180403034216-08470befbe04 // indirect
	github.com/nrdcg/auroradns v1.1.0 // indirect
	github.com/nrdcg/bunny-go v0.0.0-20250327222614-988a091fc7ea // indirect
	github.com/nrdcg/desec v0.11.0 // indirect
	github.com/nrdcg/freemyip v0.3.0 // indirect
	github.com/nrdcg/goacmedns v0.2.0 // indirect
	github.com/nrdcg/goinwx v0.11.0 // indirect
	github.com/nrdcg/mailinabox v0.2.0 // indirect
	github.com/nrdcg/namesilo v0.2.1 // indirect
	github.com/nrdcg/nodion v0.1.0 // indirect
	github.com/nrdcg/porkbun v0.4.0 // indirect
	github.com/nzdjb/go-metaname v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/opentracing/opentracing-go v1.2.1-0.20220228012449-10b1cf09e00b // indirect
	github.com/oracle/oci-go-sdk/v65 v65.94.0 // indirect
	github.com/ovh/go-ovh v1.9.0 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/peterhellberg/link v1.2.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/pquerna/otp v1.5.0 // indirect
	github.com/quic-go/qpack v0.5.1 // indirect
	github.com/regfish/regfish-dnsapi-go v0.1.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/sacloud/api-client-go v0.3.0 // indirect
	github.com/sacloud/go-http v0.1.9 // indirect
	github.com/sacloud/iaas-api-go v1.16.0 // indirect
	github.com/sacloud/packages-go v0.0.11 // indirect
	github.com/sagikazarmark/locafero v0.9.0 // indirect
	github.com/samber/lo v1.51.0 // indirect
	github.com/samber/slog-common v0.19.0 // indirect
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.33 // indirect
	github.com/selectel/domains-go v1.1.0 // indirect
	github.com/selectel/go-selvpcclient/v3 v3.2.1 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/sirupsen/logrus v1.9.4-0.20230606125235-dd1b4c2e81af // indirect
	github.com/smartystreets/go-aws-auth v0.0.0-20180515143844-0c1422d1fdb9 // indirect
	github.com/softlayer/softlayer-go v1.1.7 // indirect
	github.com/softlayer/xmlrpc v0.0.0-20200409220501-5f089df7cb7e // indirect
	github.com/sony/gobreaker v1.0.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/cast v1.9.2 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/spf13/viper v1.20.1 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.0.1196 // indirect
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/dnspod v1.0.1196 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	github.com/tklauser/go-sysconf v0.3.15 // indirect
	github.com/tklauser/numcpus v0.10.0 // indirect
	github.com/transip/gotransip/v6 v6.26.0 // indirect
	github.com/ultradns/ultradns-go-sdk v1.8.0-20241010134910-243eeec // indirect
	github.com/vinyldns/go-vinyldns v0.9.16 // indirect
	github.com/volcengine/volc-sdk-golang v1.0.213 // indirect
	github.com/vultr/govultr/v3 v3.21.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.mongodb.org/mongo-driver v1.17.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.62.0 // indirect
	go.opentelemetry.io/otel v1.37.0 // indirect
	go.opentelemetry.io/otel/metric v1.37.0 // indirect
	go.opentelemetry.io/otel/trace v1.37.0 // indirect
	go.uber.org/atomic v1.11.0
	go.uber.org/mock v0.5.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/ratelimit v0.3.1 // indirect
	golang.org/x/mod v0.25.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	golang.org/x/tools v0.34.0 // indirect
	google.golang.org/api v0.239.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/grpc v1.73.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/ns1/ns1-go.v2 v2.14.4 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/api v0.33.2 // indirect
	k8s.io/apimachinery v0.33.2 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/utils v0.0.0-20250604170112-4c0f3b243397 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.7.0 // indirect
	sigs.k8s.io/yaml v1.5.0 // indirect
)

require (
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/onsi/ginkgo/v2 v2.23.3 // indirect
	github.com/onsi/gomega v1.36.3 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
)

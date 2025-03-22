export VERSION ?= $(shell git describe --tags --abbrev=0)
export BUILD_DATE ?= $(shell date -u +'%Y%m%d-%H%M')
export GOOS = linux

LDFLAGS = -X github.com/yusing/go-proxy/pkg.version=${VERSION}


ifeq ($(agent), 1)
	NAME = godoxy-agent
	CMD_PATH = ./agent/cmd
else
	NAME = godoxy
	CMD_PATH = ./cmd
endif

ifeq ($(trace), 1)
	debug = 1
	GODOXY_TRACE ?= 1
	GODEBUG = gctrace=1 inittrace=1 schedtrace=3000
endif

ifeq ($(race), 1)
	debug = 1
  BUILD_FLAGS += -race
endif

ifeq ($(debug), 1)
	CGO_ENABLED = 0
	GODOXY_DEBUG = 1
	BUILD_FLAGS += -gcflags=all='-N -l'
endif

ifeq ($(pprof), 1)
	CGO_ENABLED = 1
	GORACE = log_path=logs/pprof strip_path_prefix=$(shell pwd)/ halt_on_error=1
	BUILD_FLAGS = -tags pprof
	VERSION := ${VERSION}-pprof
else
	CGO_ENABLED = 0
	LDFLAGS += -s -w
	BUILD_FLAGS = -pgo=auto -tags production
endif

BUILD_FLAGS += -ldflags='$(LDFLAGS)'

export NAME
export CMD_PATH
export CGO_ENABLED
export GODOXY_DEBUG
export GODOXY_TRACE
export GODEBUG
export GORACE
export BUILD_FLAGS

test:
	GODOXY_TEST=1 go test ./internal/...

get:
	go get -u ./cmd && go mod tidy

build:
	mkdir -p bin
	go build ${BUILD_FLAGS} -o bin/${NAME} ${CMD_PATH}
	if [ $(shell id -u) -eq 0 ]; \
		then setcap CAP_NET_BIND_SERVICE=+eip bin/${NAME}; \
		else sudo setcap CAP_NET_BIND_SERVICE=+eip bin/${NAME}; \
	fi

run:
	[ -f .env ] && godotenv -f .env go run ${BUILD_FLAGS} ${CMD_PATH}

mtrace:
	bin/godoxy debug-ls-mtrace > mtrace.json

rapid-crash:
	docker run --restart=always --name test_crash -p 80 debian:bookworm-slim /bin/cat &&\
	sleep 3 &&\
	docker rm -f test_crash

debug-list-containers:
	bash -c 'echo -e "GET /containers/json HTTP/1.0\r\n" | sudo netcat -U /var/run/docker.sock | tail -n +9 | jq'

ci-test:
	mkdir -p /tmp/artifacts
	act -n --artifact-server-path /tmp/artifacts -s GITHUB_TOKEN="$$(gh auth token)"

cloc:
	cloc --not-match-f '_test.go$$' cmd internal pkg

link-binary:
	ln -s /app/${NAME} bin/run

# To generate schema
# comment out this part from typescript-json-schema.js#L884
#
#	if (indexType.flags !== ts.TypeFlags.Number && !isIndexedObject) {
#			throw new Error("Not supported: IndexSignatureDeclaration with index symbol other than a number or a string");
#	}

gen-schema-single:
	bun --bun run typescript-json-schema --noExtraProps --required --skipLibCheck --tsNodeRegister=true -o schemas/${OUT} schemas/${IN} ${CLASS}
	# minify
	python3 -c "import json; f=open('schemas/${OUT}', 'r'); j=json.load(f); f.close(); f=open('schemas/${OUT}', 'w'); json.dump(j, f, separators=(',', ':'));"

gen-schema:
	bun --bun tsc
	make IN=config/config.ts \
			CLASS=Config \
			OUT=config.schema.json \
			gen-schema-single
	make IN=providers/routes.ts \
			CLASS=Routes \
			OUT=routes.schema.json \
			gen-schema-single
	make IN=middlewares/middleware_compose.ts \
			CLASS=MiddlewareCompose \
			OUT=middleware_compose.schema.json \
			gen-schema-single
	make IN=docker.ts \
			CLASS=DockerRoutes \
			OUT=docker_routes.schema.json \
			gen-schema-single

update-schema-generator:
	pnpm up -g typescript-json-schema

push-github:
	git push origin $(shell git rev-parse --abbrev-ref HEAD)
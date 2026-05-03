# POSIX sh for recipes; ignore login $SHELL (e.g. fish) — recipes use sh syntax.
SHELL := /bin/sh
export VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null)
export BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
export BUILD_DATE ?= $(shell date -u +'%Y%m%d-%H%M')
export GOOS = linux

REPO_URL ?= https://github.com/yusing/godoxy

WEBUI_DIR ?= $(shell pwd)/webui
DOCS_DIR ?= ${WEBUI_DIR}/wiki

TEST_REGISTRY ?= reg.i.sh

GO_TAGS =

LDFLAGS = -X github.com/yusing/goutils/version.version=${VERSION} -checklinkname=0

PACKAGE ?= ./cmd

ifeq ($(agent), 1)
	NAME = godoxy-agent
	PWD = ${shell pwd}/agent
else ifeq ($(socket-proxy), 1)
	NAME = godoxy-socket-proxy
	PWD = ${shell pwd}/socket-proxy
else ifeq ($(cli), 1)
	NAME = godoxy-cli
	PWD = ${shell pwd}/cmd/cli
	PACKAGE = .
else
	NAME = godoxy
	PWD = ${shell pwd}
	godoxy = 1
endif

ifeq ($(trace), 1)
	debug = 1
	GODOXY_TRACE ?= 1
	GODEBUG = gctrace=1 inittrace=1 schedtrace=3000
endif

ifeq ($(race), 1)
	CGO_ENABLED = 1
	GODOXY_DEBUG = 1
	GO_TAGS += debug
	BUILD_FLAGS += -race
else ifeq ($(debug), 1)
	CGO_ENABLED = 1
	GODOXY_DEBUG = 1
	GO_TAGS += debug
	# FIXME: BUILD_FLAGS += -asan -gcflags=all='-N -l'
else ifeq ($(dev), 1)
	CGO_ENABLED = 0
	GODOXY_DEBUG = 1
else ifeq ($(pprof), 1)
	CGO_ENABLED = 0
	GORACE = log_path=logs/pprof strip_path_prefix=$(shell pwd)/ halt_on_error=1
	GO_TAGS += pprof
	VERSION := ${VERSION}-pprof
else
	CGO_ENABLED = 0
	LDFLAGS += -s -w
	GO_TAGS += production
	BUILD_FLAGS += -pgo=auto
endif

BUILD_FLAGS += -tags '$(GO_TAGS)' -ldflags='$(LDFLAGS)'
BIN_PATH := $(shell pwd)/bin/${NAME}

export NAME
export CGO_ENABLED
export GODOXY_DEBUG
export GODOXY_TRACE
export GODEBUG
export GORACE
export BUILD_FLAGS

ifeq ($(shell id -u), 0)
	SETCAP_CMD = setcap
else
	SETCAP_CMD = sudo setcap
endif


# CAP_NET_BIND_SERVICE: permission for binding to :80 and :443
POST_BUILD = echo;

ifeq ($(godoxy), 1)
	POST_BUILD += $(SETCAP_CMD) CAP_NET_BIND_SERVICE=+ep ${BIN_PATH};
endif
ifeq ($(docker), 1)
	POST_BUILD += mkdir -p /app && mv ${BIN_PATH} /app/run;
endif

.PHONY: benchmark build build-cli build-webui ci-test cloc debug debug-list-containers \
	dev docker-build-test ensure-webui-dist gen-api-types gen-cli gen-swagger help \
	minify mod-tidy modernize push-github rapid-crash run tcp-echo-test test \
	update-deps update-go update-wiki

# Show usage (like CLI --help). Run: make help
help:
	@printf '%s\n' \
		'Godoxy Makefile' '' \
		'Usage: make <target> [VAR=value ...]' '' \
		'Build which binary (default: godoxy main server):' \
		'  agent=1          godoxy-agent (agent/)' \
		'  socket-proxy=1   godoxy-socket-proxy (socket-proxy/)' \
		'  cli=1            godoxy-cli (cmd/cli/)' '' \
		'Build / run flags (combine as needed):' \
		'  debug=1          debug build (CGO, GODOXY_DEBUG)' \
		'  race=1           -race tests/build' \
		'  pprof=1          pprof tag + GORACE settings' \
		'  trace=1          trace + gctrace (implies debug=1)' \
		'  docker=1         post-build: move binary into /app for image' '' \
		'Targets:' \
		'  help             Show this message' \
		'  test             go test -race internal/...' \
		'  tcp-echo-test    scripts/tcp_echo_test.ts' \
		'  docker-build-test  Build/push godoxy docker images' \
		'  update-go        Bump Go version in mods + Dockerfiles + tidy' \
		'  update-deps      go get -u + tidy across modules' \
		'  mod-tidy         go mod tidy across modules' \
		'  modernize        go fix ./... across modules' \
		'  minify           Frontend minify (skipped for agent/socket-proxy)' \
		"  build            Produce bin/$(NAME)" \
		'  run              godotenv + go run main package' \
		'  dev              docker compose -f dev.compose.yml $$(args)' \
		'  dev-build        build then compose up app --force-recreate' \
		'  benchmark        Run scripts/benchmark.sh; TARGET/compose handled by script; extra args via args=' \
		'  rapid-crash      Short docker restart loop sanity check' \
		'  debug-list-containers  Docker socket GET /containers/json via netcat+jq' \
		'  ci-test          act dry-run with artifacts path' \
		'  cloc             scc on Go sources and WebUI source' \
		"  push-github      git push origin $(BRANCH)" \
		'  gen-swagger      swag init + scripts into internal/api docs' \
		'  gen-api-types    gen-swagger + swagger-typescript-api for webui' \
		'  gen-cli          Regenerate CLI (cmd/cli gen)' \
		'  update-wiki      bun scripts/update-wiki (WEBUI_DIR, REPO_URL)'

ensure-webui-dist:
	@if [ "${godoxy}" = "1" ] && [ ! -f "$(WEBUI_DIR)/dist/client/_shell.html" ]; then \
		$(MAKE) build-webui; \
	fi

test: ensure-webui-dist
	CGO_ENABLED=1 go test -v -race ${BUILD_FLAGS} ./internal/...

tcp-echo-test:
	bun --bun scripts/tcp_echo_test.ts

docker-build-test:
	docker build --target=main -t ${TEST_REGISTRY}/godoxy .
	docker build --target=agent --build-arg=MAKE_ARGS=agent=1 -t ${TEST_REGISTRY}/godoxy-agent .
	docker build --target=socket-proxy --build-arg=MAKE_ARGS=socket-proxy=1 -t ${TEST_REGISTRY}/godoxy-socket-proxy .
	docker push ${TEST_REGISTRY}/godoxy
	docker push ${TEST_REGISTRY}/godoxy-agent
	docker push ${TEST_REGISTRY}/godoxy-socket-proxy

go_ver := $(shell go version | cut -d' ' -f3 | cut -d'o' -f2)
files := $(shell find . -name go.mod -type f -or -name Dockerfile -type f)
gomod_paths := $(shell find . -name go.mod -type f | grep -vE '^./internal/(go-oidc|go-proxmox|gopsutil)/' | sed 's#/go\.mod$$##')

update-go:
	@for file in ${files}; do \
		echo "updating $$file"; \
		sed -i 's|go \([0-9]\+\.[0-9]\+\.[0-9]\+\)|go ${go_ver}|g' $$file; \
		sed -i 's|FROM golang:.*-alpine|FROM golang:${go_ver}-alpine|g' $$file; \
	done
	@for path in ${gomod_paths}; do \
		echo ${PWD}/$$path && cd ${PWD}/$$path && go mod tidy; \
	done

update-deps:
	@for path in ${gomod_paths}; do \
		echo ${PWD}/$$path && cd ${PWD}/$$path && go get -u ./... && go mod tidy; \
	done

mod-tidy:
	@for path in ${gomod_paths}; do \
		echo ${PWD}/$$path && cd ${PWD}/$$path && go mod tidy; \
	done

modernize:
	@for path in ${gomod_paths}; do \
		echo ${PWD}/$$path && cd ${PWD}/$$path && go fix ./...; \
	done

minify:
	@if [ "${agent}" = "1" ]; then \
		echo "minify: skipped for agent"; \
	elif [ "${socket-proxy}" = "1" ]; then \
		echo "minify: skipped for socket-proxy"; \
	else \
		bun --bun scripts/minify; \
	fi

build-webui:
	cd "$(WEBUI_DIR)" && \
	bun i --frozen-lockfile && \
	$(MAKE) gen-schema && \
	node ./node_modules/vite/bin/vite.js build

build: ensure-webui-dist
	@if [ "${godoxy}" = "1" ]; then \
		make minify; \
	elif [ "${cli}" = "1" ]; then \
		make gen-cli; \
	fi
	mkdir -p $(shell dirname ${BIN_PATH})
	go build -C ${PWD} ${BUILD_FLAGS} -o ${BIN_PATH} ${PACKAGE}
	${POST_BUILD}

run: minify
	cd ${PWD} && [ -f .env ] && godotenv -f .env go run ${BUILD_FLAGS} ${PACKAGE}

dev:
	$(MAKE) dev=1 build
	docker compose -f dev.compose.yml up -t 0 -d app --force-recreate
	docker compose logs -f -n200 app

benchmark:
	./scripts/benchmark.sh $(args)

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
	scc -w -i go,ts,tsx --not-match '_test.go$$' --not-match '.test.ts$$' --not-match '.test.tsx$$'

push-github:
	git push origin $(BRANCH)

gen-swagger:
  # go install github.com/swaggo/swag/cmd/swag@latest
	swag init --parseDependency --parseInternal --parseFuncBody -g handler.go -d internal/api -o internal/api/v1/docs
	python3 scripts/fix-swagger-json.py
	# we don't need this
	rm internal/api/v1/docs/docs.go
	cp internal/api/v1/docs/swagger.json ${DOCS_DIR}/public/api.json

gen-api-types: gen-swagger
	# --disable-throw-on-error
	bunx --bun swagger-typescript-api generate --sort-types --generate-union-enums --add-readonly --route-types \
	--responses -o ${WEBUI_DIR}/src/lib -n api.ts -p internal/api/v1/docs/swagger.json

gen-cli:
	cd cmd/cli && go run ./gen

update-wiki:
	DOCS_DIR=${DOCS_DIR} REPO_URL=${REPO_URL} bun --bun scripts/update-wiki

shell := /bin/sh
export VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null)
export BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
export BUILD_DATE ?= $(shell date -u +'%Y%m%d-%H%M')
export GOOS = linux

REPO_URL ?= https://github.com/yusing/godoxy

WEBUI_DIR ?= $(shell pwd)/../godoxy-webui
DOCS_DIR ?= ${WEBUI_DIR}/wiki

TEST_REGISTRY ?= reg.i.sh

ifneq ($(BRANCH), compat)
	GO_TAGS = sonic
else
	GO_TAGS =
endif

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
AUTOCERT_BIN_PATH := $(shell pwd)/bin/autocert

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
	ifeq ($(godoxy), 1)
		POST_BUILD += mv ${AUTOCERT_BIN_PATH} /app/autocert;
	endif
endif

.PHONY: debug

test:
	CGO_ENABLED=1 go test -v -race ${BUILD_FLAGS} ./internal/...

docker-build-test:
	docker build -t ${TEST_REGISTRY}/godoxy .
	docker build --build-arg=MAKE_ARGS=agent=1 -t ${TEST_REGISTRY}/godoxy-agent .
	docker build --build-arg=MAKE_ARGS=socket-proxy=1 -t ${TEST_REGISTRY}/godoxy-socket-proxy .
	docker push ${TEST_REGISTRY}/godoxy
	docker push ${TEST_REGISTRY}/godoxy-agent
	docker push ${TEST_REGISTRY}/godoxy-socket-proxy

go_ver := $(shell go version | cut -d' ' -f3 | cut -d'o' -f2)
files := $(shell find . -name go.mod -type f -or -name Dockerfile -type f)
gomod_paths := $(shell find . -name go.mod -type f | grep -vE '^./internal/(go-oidc|go-proxmox|gopsutil)/' | xargs dirname)

update-go:
	for file in ${files}; do \
		echo "updating $$file"; \
		sed -i 's|go \([0-9]\+\.[0-9]\+\.[0-9]\+\)|go ${go_ver}|g' $$file; \
		sed -i 's|FROM golang:.*-alpine|FROM golang:${go_ver}-alpine|g' $$file; \
	done
	for path in ${gomod_paths}; do \
		cd ${PWD}/$$path && go mod tidy; \
	done

update-deps:
	for path in ${gomod_paths}; do \
		cd ${PWD}/$$path && go get -u ./... && go mod tidy; \
	done

mod-tidy:
	for path in ${gomod_paths}; do \
		cd ${PWD}/$$path && go mod tidy; \
	done

modernize:
	for path in ${gomod_paths}; do \
		cd ${PWD}/$$path && go fix ./...; \
	done

minify:
	@if [ "${agent}" = "1" ]; then \
		echo "minify: skipped for agent"; \
	elif [ "${socket-proxy}" = "1" ]; then \
		echo "minify: skipped for socket-proxy"; \
	else \
		bun --bun scripts/minify; \
	fi

build:
	@if [ "${godoxy}" = "1" ]; then \
		make minify; \
	elif [ "${cli}" = "1" ]; then \
		make gen-cli; \
	fi
	mkdir -p $(shell dirname ${BIN_PATH})
	go build -C ${PWD} ${BUILD_FLAGS} -o ${BIN_PATH} ${PACKAGE}
	@if [ "${godoxy}" = "1" ]; then \
		go build -C ${shell pwd}/cmd/autocert ${BUILD_FLAGS} -o ${AUTOCERT_BIN_PATH} .; \
	fi
	${POST_BUILD}

run: minify
	cd ${PWD} && [ -f .env ] && godotenv -f .env go run ${BUILD_FLAGS} ${PACKAGE}

dev:
	docker compose -f dev.compose.yml $(args)

dev-build: build
	docker compose -f dev.compose.yml up -t 0 -d app --force-recreate

benchmark:
	@TARGETS="$(TARGET)"; \
	if [ -z "$$TARGETS" ]; then TARGETS="godoxy traefik caddy nginx"; fi; \
	trap 'docker compose -f dev.compose.yml down $$TARGETS' EXIT; \
	docker compose -f dev.compose.yml up -d --force-recreate $$TARGETS; \
	sleep 1; \
	./scripts/benchmark.sh

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
	scc -w -i go --not-match '_test.go$$'

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

.PHONY: gen-cli build-cli update-wiki

gen-cli:
	cd cmd/cli && go run ./gen

update-wiki:
	DOCS_DIR=${DOCS_DIR} REPO_URL=${REPO_URL} bun --bun scripts/update-wiki

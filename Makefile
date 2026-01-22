shell := /bin/sh
export VERSION ?= $(shell git describe --tags --abbrev=0)
export BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)
export BUILD_DATE ?= $(shell date -u +'%Y%m%d-%H%M')
export GOOS = linux

REPO_URL ?= https://github.com/yusing/godoxy

WEBUI_DIR ?= ../godoxy-webui
DOCS_DIR ?= ${WEBUI_DIR}/wiki

ifneq ($(BRANCH), compat)
	GO_TAGS = sonic
else
	GO_TAGS =
endif

LDFLAGS = -X github.com/yusing/goutils/version.version=${VERSION} -checklinkname=0

ifeq ($(agent), 1)
	NAME = godoxy-agent
	PWD = ${shell pwd}/agent
else ifeq ($(socket-proxy), 1)
	NAME = godoxy-socket-proxy
	PWD = ${shell pwd}/socket-proxy
else
	NAME = godoxy
	PWD = ${shell pwd}
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
POST_BUILD = $(SETCAP_CMD) CAP_NET_BIND_SERVICE=+ep ${BIN_PATH};
ifeq ($(docker), 1)
	POST_BUILD += mkdir -p /app && mv ${BIN_PATH} /app/run;
endif

.PHONY: debug

test:
	CGO_ENABLED=1 go test -v -race ${BUILD_FLAGS} ./internal/...

docker-build-test:
	docker build -t godoxy .
	docker build --build-arg=MAKE_ARGS=agent=1 -t godoxy-agent .
	docker build --build-arg=MAKE_ARGS=socket-proxy=1 -t godoxy-socket-proxy .

go_ver := $(shell go version | cut -d' ' -f3 | cut -d'o' -f2)
files := $(shell find . -name go.mod -type f -or -name Dockerfile -type f)
gomod_paths := $(shell find . -name go.mod -type f | xargs dirname)

update-go:
	for file in ${files}; do \
		echo "updating $$file"; \
		sed -i 's|go \([0-9]\+\.[0-9]\+\.[0-9]\+\)|go ${go_ver}|g' $$file; \
		sed -i 's|FROM golang:.*-alpine|FROM golang:${go_ver}-alpine|g' $$file; \
	done
	for path in ${gomod_paths}; do \
		echo "go mod tidy $$path"; \
		cd ${PWD}/$$path && go mod tidy; \
	done

update-deps:
	for path in ${gomod_paths}; do \
		echo "go get -u $$path"; \
		cd ${PWD}/$$path && go get -u ./... && go mod tidy; \
	done

mod-tidy:
	for path in ${gomod_paths}; do \
		echo "go mod tidy $$path"; \
		cd ${PWD}/$$path && go mod tidy; \
	done

build:
	mkdir -p $(shell dirname ${BIN_PATH})
	go build -C ${PWD} ${BUILD_FLAGS} -o ${BIN_PATH} ./cmd
	${POST_BUILD}

run:
	cd ${PWD} && [ -f .env ] && godotenv -f .env go run ${BUILD_FLAGS} ./cmd

dev:
	docker compose -f dev.compose.yml $(args)

dev-build: build
	docker compose -f dev.compose.yml up -t 0 -d app --force-recreate

benchmark:
	@if [ -z "$(TARGET)" ]; then \
		docker compose -f dev.compose.yml up -d --force-recreate godoxy traefik caddy nginx; \
	else \
		docker compose -f dev.compose.yml up -d --force-recreate $(TARGET); \
	fi
	sleep 1
	@./scripts/benchmark.sh

dev-run: build
	cd dev-data && ${BIN_PATH}

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

gen-swagger-markdown: gen-swagger
  # brew tap go-swagger/go-swagger && brew install go-swagger
	swagger generate markdown -f internal/api/v1/docs/swagger.yaml --skip-validation --output ${DOCS_DIR}/src/API.md

gen-api-types: gen-swagger
	# --disable-throw-on-error
	bunx --bun swagger-typescript-api generate --sort-types --generate-union-enums --axios --add-readonly --route-types \
		 --responses -o ${WEBUI_DIR}/lib -n api.ts -p internal/api/v1/docs/swagger.json
	bunx --bun prettier --config ${WEBUI_DIR}/.prettierrc --write ${WEBUI_DIR}/lib/api.ts

.PHONY: update-wiki
update-wiki:
	DOCS_DIR=${DOCS_DIR} REPO_URL=${REPO_URL} bun --bun scripts/update-wiki/main.ts

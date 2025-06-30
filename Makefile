shell := /bin/sh
export VERSION ?= $(shell git describe --tags --abbrev=0)
export BUILD_DATE ?= $(shell date -u +'%Y%m%d-%H%M')
export GOOS = linux

LDFLAGS = -X github.com/yusing/go-proxy/pkg.version=${VERSION}


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
	debug = 1
  BUILD_FLAGS += -race
endif

ifeq ($(debug), 1)
	CGO_ENABLED = 1
	GODOXY_DEBUG = 1
	BUILD_FLAGS += -gcflags=all='-N -l' -tags debug -asan
else ifeq ($(pprof), 1)
	CGO_ENABLED = 1
	GORACE = log_path=logs/pprof strip_path_prefix=$(shell pwd)/ halt_on_error=1
	BUILD_FLAGS += -tags pprof
	VERSION := ${VERSION}-pprof
else
	CGO_ENABLED = 0
	LDFLAGS += -s -w
	BUILD_FLAGS += -pgo=auto -tags production
endif

BUILD_FLAGS += -ldflags='$(LDFLAGS)'
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
	GODOXY_TEST=1 go test ./internal/...

docker-build-test:
	docker build -t godoxy .
	docker build --build-arg=MAKE_ARGS=agent=1 -t godoxy-agent .

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
	cd ${PWD} && go build ${BUILD_FLAGS} -o ${BIN_PATH} ./cmd
	${POST_BUILD}

run:
	cd ${PWD} && [ -f .env ] && godotenv -f .env go run ${BUILD_FLAGS} ./cmd

debug:
	make NAME="godoxy-test" debug=1 build
	sh -c 'HTTP_ADDR=:81 HTTPS_ADDR=:8443 API_ADDR=:8899 DEBUG=1 bin/godoxy-test'

mtrace:
	 ${BIN_PATH} debug-ls-mtrace > mtrace.json

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
	cloc --include-lang=Go --not-match-f '_test.go$$' .

push-github:
	git push origin $(shell git rev-parse --abbrev-ref HEAD)
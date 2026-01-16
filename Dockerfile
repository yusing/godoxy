# Stage 1: deps
FROM golang:1.25.6-alpine AS deps
HEALTHCHECK NONE

# package version does not matter
# trunk-ignore(hadolint/DL3018)
RUN apk add --no-cache tzdata make libcap-setcap

ENV GOPATH=/root/go
ENV GOCACHE=/root/.cache/go-build

WORKDIR /src

COPY goutils/go.mod goutils/go.sum ./goutils/
COPY internal/go-oidc/go.mod internal/go-oidc/go.sum ./internal/go-oidc/
COPY internal/gopsutil/go.mod internal/gopsutil/go.sum ./internal/gopsutil/
COPY go.mod go.sum ./

# remove godoxy stuff from go.mod first
RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/go/pkg/mod \
  sed -i '/^module github\.com\/yusing\/godoxy/!{/github\.com\/yusing\/godoxy/d}' go.mod && \
  sed -i '/^module github\.com\/yusing\/goutils/!{/github\.com\/yusing\/goutils/d}' go.mod && \
  go mod download -x

# Stage 2: builder
FROM deps AS builder

WORKDIR /src

COPY go.mod go.sum ./
COPY Makefile ./
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
COPY agent ./agent
COPY socket-proxy ./socket-proxy
COPY goutils ./goutils

ARG VERSION
ENV VERSION=${VERSION}

ARG MAKE_ARGS
ENV MAKE_ARGS=${MAKE_ARGS}

RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/go/pkg/mod \
  make ${MAKE_ARGS} docker=1 build

# Stage 3: Final image
FROM scratch

LABEL maintainer="yusing@6uo.me"
LABEL proxy.exclude=1
LABEL proxy.#1.healthcheck.disable=true

# copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# copy binary
COPY --from=builder /app/run /app/run

# copy certs
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

ENV DOCKER_HOST=unix:///var/run/docker.sock

WORKDIR /app

CMD ["/app/run"]
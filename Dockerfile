# Stage 1: deps
FROM golang:1.24.5-alpine AS deps
HEALTHCHECK NONE

# package version does not matter
# trunk-ignore(hadolint/DL3018)
RUN apk add --no-cache tzdata make libcap-setcap

ENV GOPATH=/root/go

WORKDIR /src

COPY go.mod go.sum ./

# remove godoxy stuff from go.mod first
RUN sed -i '/^module github\.com\/yusing\/go-proxy/!{/github\.com\/yusing\/go-proxy/d}' go.mod && \
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

ARG VERSION
ENV VERSION=${VERSION}

ARG MAKE_ARGS
ENV MAKE_ARGS=${MAKE_ARGS}

ENV GOCACHE=/root/.cache/go-build
ENV GOPATH=/root/go

RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/go/pkg/mod \
  make ${MAKE_ARGS} docker=1 build

# Stage 3: Final image
FROM scratch

LABEL maintainer="yusing@6uo.me"
LABEL proxy.exclude=1

# copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# copy binary
COPY --from=builder /app/run /app/run

# copy certs
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

ENV DOCKER_HOST=unix:///var/run/docker.sock

WORKDIR /app

CMD ["/app/run"]
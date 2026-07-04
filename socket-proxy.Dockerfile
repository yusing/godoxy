# Stage 1: deps
FROM golang:1.26.4-alpine AS deps
HEALTHCHECK NONE

# package version does not matter
# trunk-ignore(hadolint/DL3018)
RUN apk add --no-cache tzdata libcap-setcap

ARG SHADOWTREE_VERSION=latest
RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/go/pkg/mod \
  CGO_ENABLED=0 go install github.com/yusing/shadowtree/cmd/shadowtree@${SHADOWTREE_VERSION}

ENV GOPATH=/root/go

WORKDIR /src

COPY socket-proxy/go.mod socket-proxy/go.sum ./

RUN sed -i '/^module github\.com\/yusing\/goutils/!{/github\.com\/yusing\/goutils/d}' go.mod && \
    go mod download -x

# Stage 2: builder
FROM deps AS builder

WORKDIR /src

COPY .shadowtree.toml ./
COPY socket-proxy ./socket-proxy
COPY goutils ./goutils

ARG VERSION
ENV VERSION=${VERSION}

ARG SHADOWTREE_ARGS
ENV SHADOWTREE_ARGS=${SHADOWTREE_ARGS}

ENV GOCACHE=/root/.cache/go-build
ENV GOPATH=/root/go

RUN shadowtree build ${SHADOWTREE_ARGS:-component=socket-proxy} docker=true

# Stage 3: Final image
FROM scratch AS socket-proxy

LABEL maintainer="yusing@6uo.me"
LABEL proxy.exclude=1

# copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# copy binary
COPY --from=builder /app/run /app/run

WORKDIR /app

LABEL proxy.#1.healthcheck.disable=true

ENV LISTEN_ADDR=0.0.0.0:2375
CMD ["/app/run"]

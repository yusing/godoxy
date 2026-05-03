# Stage 1: go-base
FROM golang:1.26.2-alpine AS go-base
HEALTHCHECK NONE

# package version does not matter
# trunk-ignore(hadolint/DL3018)
RUN apk add --no-cache tzdata make libcap-setcap

# Stage 2: frontend-base
FROM go-base AS frontend-base

# libgcc and libstdc++ are needed for bun
# trunk-ignore(hadolint/DL3018)
RUN apk add --no-cache libgcc libstdc++

# for minify and webui build
COPY --from=oven/bun:1-alpine /usr/local/bin/bun /usr/local/bin/bun
COPY --from=oven/bun:1-alpine /usr/local/bin/bunx /usr/local/bin/bunx
COPY --from=node:lts-alpine3.22 /usr/local/bin/node /usr/local/bin/node
COPY --from=node:lts-alpine3.22 /usr/local/bin/npm /usr/local/bin/npm

# Stage 3: godoxy deps
FROM go-base AS godoxy-deps

ENV GOPATH=/root/go
ENV GOCACHE=/root/.cache/go-build

WORKDIR /src

COPY goutils/go.mod goutils/go.sum ./goutils/
COPY internal/go-oidc/go.mod internal/go-oidc/go.sum ./internal/go-oidc/
COPY internal/gopsutil/go.mod internal/gopsutil/go.sum ./internal/gopsutil/
COPY internal/go-proxmox/go.mod internal/go-proxmox/go.sum ./internal/go-proxmox/
COPY go.mod go.sum ./

# remove godoxy stuff from go.mod first
RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/go/pkg/mod \
  sed -i '/^module github\.com\/yusing\/godoxy/!{/github\.com\/yusing\/godoxy/d}' go.mod && \
  sed -i '/^module github\.com\/yusing\/goutils/!{/github\.com\/yusing\/goutils/d}' go.mod && \
  go mod download -x

# Stage 4: webui deps
FROM frontend-base AS webui-deps

WORKDIR /src

COPY webui/package.json webui/bun.lock ./

RUN bun install --frozen-lockfile

# Stage 5: webui schema generation
FROM frontend-base AS webui-schema

WORKDIR /src

COPY webui/src/types/godoxy/ ./src/types/godoxy/
COPY webui/Makefile ./Makefile
COPY webui/tsconfig.json ./tsconfig.json

RUN --mount=type=cache,target=/root/.bun make gen-schema

# Stage 6: webui build
FROM frontend-base AS webui-build

WORKDIR /src

COPY --from=webui-deps /src/node_modules ./node_modules
COPY webui .
COPY --from=webui-schema /src/src/types/godoxy/*.json ./src/types/godoxy/

ENV NODE_ENV=production
RUN node ./node_modules/vite/bin/vite.js build

# Stage 7: binary source
FROM godoxy-deps AS binary-source

WORKDIR /src

COPY scripts/minify ./scripts/minify
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

ARG BRANCH
ENV BRANCH=${BRANCH}

ENV GOPATH=/root/go
ENV GOCACHE=/root/.cache/go-build

# Stage 8: non-main builder
FROM binary-source AS non-main-builder

RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/go/pkg/mod \
  make ${MAKE_ARGS} docker=1 build

# Stage 9: main builder
FROM binary-source AS main-builder

# libgcc and libstdc++ are needed for bun
# trunk-ignore(hadolint/DL3018)
RUN apk add --no-cache libgcc libstdc++

COPY --from=oven/bun:1-alpine /usr/local/bin/bun /usr/local/bin/bun
COPY --from=oven/bun:1-alpine /usr/local/bin/bunx /usr/local/bin/bunx
COPY --from=node:lts-alpine3.22 /usr/local/bin/node /usr/local/bin/node
COPY --from=node:lts-alpine3.22 /usr/local/bin/npm /usr/local/bin/npm
COPY webui/embed.go ./webui/embed.go
COPY webui/embed_dev.go ./webui/embed_dev.go
COPY --from=webui-build /src/dist/client ./webui/dist/client

RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/root/go/pkg/mod \
  make ${MAKE_ARGS} docker=1 build

# Stage 10: agent image
FROM scratch AS agent

LABEL maintainer="yusing@6uo.me"
LABEL proxy.exclude=1
LABEL proxy.#1.healthcheck.disable=true

COPY --from=non-main-builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=non-main-builder /app/run /app/run
COPY --from=non-main-builder /etc/ssl/certs /etc/ssl/certs

ENV DOCKER_HOST=unix:///var/run/docker.sock

WORKDIR /app

CMD ["/app/run"]

# Stage 11: socket proxy image
FROM scratch AS socket-proxy

LABEL maintainer="yusing@6uo.me"
LABEL proxy.exclude=1
LABEL proxy.#1.healthcheck.disable=true

COPY --from=non-main-builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=non-main-builder /app/run /app/run
COPY --from=non-main-builder /etc/ssl/certs /etc/ssl/certs

ENV LISTEN_ADDR=0.0.0.0:2375

WORKDIR /app

CMD ["/app/run"]

# Stage 12: main image
FROM scratch AS main

LABEL maintainer="yusing@6uo.me"
LABEL proxy.exclude=1
LABEL proxy.#1.healthcheck.disable=true

COPY --from=main-builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=main-builder /app/run /app/run
COPY --from=main-builder /etc/ssl/certs /etc/ssl/certs

ENV DOCKER_HOST=unix:///var/run/docker.sock

WORKDIR /app

CMD ["/app/run"]

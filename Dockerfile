# Stage 1: utils-deps
FROM golang:1.26.2-alpine AS utils-deps
HEALTHCHECK NONE

# package version does not matter
# libgcc and libstdc++ are needed for bun
# trunk-ignore(hadolint/DL3018)
RUN apk add --no-cache tzdata make libcap-setcap libgcc libstdc++

# for minify and webui build
COPY --from=oven/bun:1-alpine /usr/local/bin/bun /usr/local/bin/bun
COPY --from=oven/bun:1-alpine /usr/local/bin/bunx /usr/local/bin/bunx
COPY --from=node:lts-alpine3.22 /usr/local/bin/node /usr/local/bin/node
COPY --from=node:lts-alpine3.22 /usr/local/bin/npm /usr/local/bin/npm

# Stage 2: godoxy deps

FROM utils-deps AS godoxy-deps

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

# Stage 3: webui deps

FROM utils-deps AS webui-deps

WORKDIR /src

COPY webui/package.json webui/bun.lock ./

RUN bun install --frozen-lockfile

# Stage 4: webui schema generation

FROM utils-deps AS webui-schema

WORKDIR /src

COPY webui/src/types/godoxy/ ./src/types/godoxy/
COPY webui/Makefile ./Makefile
COPY webui/tsconfig.json ./tsconfig.json

RUN --mount=type=cache,target=/root/.bun make gen-schema

# Stage 5: webui build

FROM utils-deps AS webui-build

WORKDIR /src

COPY --from=webui-deps /src/node_modules ./node_modules
COPY webui .
COPY --from=webui-schema /src/src/types/godoxy/*.json ./src/types/godoxy/

ENV NODE_ENV=production
RUN node ./node_modules/vite/bin/vite.js build

# Stage 6: godoxy builder
FROM godoxy-deps AS builder

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
COPY webui/embed.go ./webui/embed.go
COPY --from=webui-build /src/dist/client ./webui/dist/client

ARG VERSION
ENV VERSION=${VERSION}

ARG MAKE_ARGS
ENV MAKE_ARGS=${MAKE_ARGS}

ARG BRANCH
ENV BRANCH=${BRANCH}

ENV GOPATH=/root/go
ENV GOCACHE=/root/.cache/go-build

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

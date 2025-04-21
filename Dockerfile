# Stage 1: deps
FROM golang:1.24.2-alpine AS deps
HEALTHCHECK NONE

# package version does not matter
# trunk-ignore(hadolint/DL3018)
RUN apk add --no-cache tzdata make libcap-setcap

WORKDIR /src

# Only copy go.mod and go.sum initially for better caching
COPY go.mod go.sum /src/

ENV GOPATH=/root/go
RUN go mod download -x

# Stage 2: builder
FROM deps AS builder

WORKDIR /src

COPY Makefile ./
COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg
COPY agent ./agent

ARG VERSION
ENV VERSION=${VERSION}

ARG MAKE_ARGS
ENV MAKE_ARGS=${MAKE_ARGS}

ENV GOCACHE=/root/.cache/go-build
ENV GOPATH=/root/go
RUN make ${MAKE_ARGS} build link-binary && \
    mv bin /app/ && \
    mkdir -p /app/error_pages /app/certs

# Stage 3: Final image
FROM scratch

LABEL maintainer="yusing@6uo.me"
LABEL proxy.exclude=1

# copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# copy binary
COPY --from=builder /app /app

# copy example config
COPY config.example.yml /app/config/config.yml

# copy certs
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

ENV DOCKER_HOST=unix:///var/run/docker.sock

WORKDIR /app

CMD ["/app/run"]
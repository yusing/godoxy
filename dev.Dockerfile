# Stage 1: deps
FROM golang:1.25.0-alpine AS deps
HEALTHCHECK NONE

# package version does not matter
# trunk-ignore(hadolint/DL3018)
RUN apk add --no-cache tzdata make libcap-setcap

# Stage 3: Final image
FROM alpine:3.22

LABEL maintainer="yusing@6uo.me"
LABEL proxy.exclude=1

# copy timezone data
COPY --from=deps /usr/share/zoneinfo /usr/share/zoneinfo

# copy certs
COPY --from=deps /etc/ssl/certs /etc/ssl/certs

ARG TARGET
ENV TARGET=${TARGET}

ENV DOCKER_HOST=unix:///var/run/docker.sock

# copy binary
COPY bin/${TARGET} /app/run

WORKDIR /app

RUN chown -R 1000:1000 /app

CMD ["/app/run"]
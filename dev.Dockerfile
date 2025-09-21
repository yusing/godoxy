# Stage 1: deps
FROM alpine:3.22 AS deps
HEALTHCHECK NONE

# package version does not matter
# trunk-ignore(hadolint/DL3018)
RUN apk add --no-cache tzdata

# Stage 2: Final image
FROM deps

LABEL maintainer="yusing@6uo.me"
LABEL proxy.exclude=1

ARG TARGET
ENV TARGET=${TARGET}

ENV DOCKER_HOST=unix:///var/run/docker.sock

# copy binary
COPY bin/${TARGET} /app/run

WORKDIR /app

RUN chown -R 1000:1000 /app

CMD ["/app/run"]
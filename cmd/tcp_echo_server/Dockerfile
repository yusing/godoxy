FROM golang:1.26.2-alpine AS builder

HEALTHCHECK NONE

WORKDIR /src

COPY main.go ./

RUN go build -o tcp_echo_server main.go

FROM scratch

COPY --from=builder /src/tcp_echo_server /app/run

USER 1001:1001

EXPOSE 9000/tcp 9443/tcp

CMD ["/app/run"]

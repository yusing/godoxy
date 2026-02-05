FROM golang:1.25.7-alpine AS builder

HEALTHCHECK NONE

WORKDIR /src

COPY go.mod go.sum ./
COPY main.go ./

RUN go build -o h2c_test_server main.go

FROM scratch

COPY --from=builder /src/h2c_test_server /app/run

USER 1001:1001

CMD ["/app/run"]
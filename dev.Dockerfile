FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /app

CMD ["/app/run"]

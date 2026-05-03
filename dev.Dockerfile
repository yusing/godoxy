FROM oven/bun:1-slim

RUN apt-get update && apt-get install -y ca-certificates

WORKDIR /app

CMD ["/app/run"]

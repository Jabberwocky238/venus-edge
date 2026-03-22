FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/master ./operator/master/cmd
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/agent ./operator/agent/cmd

FROM debian:bookworm-slim AS runtime-base

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/master /usr/local/bin/master
COPY --from=builder /out/agent /usr/local/bin/agent

EXPOSE 9000 10992 8443 8080 8053

CMD ["master"]

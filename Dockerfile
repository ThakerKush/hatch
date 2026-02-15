FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /hatchd ./cmd/hatchd

# -------------------------------------------------------------------
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates \
        dnsmasq-base \
        iproute2 \
        iptables \
        procps \
        e2fsprogs \
        curl && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /hatchd /usr/local/bin/hatchd

RUN mkdir -p /data
ENV HATCH_DATA_DIR=/data

EXPOSE 8080 9090

ENTRYPOINT ["hatchd"]

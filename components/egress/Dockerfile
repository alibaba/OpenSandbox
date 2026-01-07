FROM golang:1.24-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./

# Static-ish build (no cgo) to simplify runtime deps
ENV CGO_ENABLED=0
RUN go mod download

COPY . .
RUN go build -o /out/egress .

FROM debian:bookworm-slim

# iptables is needed for DNS REDIRECT; ca-certificates for TLS to upstream resolvers
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        iptables \
        ca-certificates \
        curl \
        wget \
        net-tools \
        dnsutils \
        netcat-openbsd \
        iputils-ping \
        traceroute \
        telnet \
        tcpdump \
        nmap \
        htop \
        procps \
        strace \
        lsof \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/egress /egress

# Default entrypoint; expects OPENSANDBOX_NETWORK_POLICY env at runtime.
ENTRYPOINT ["/egress"]


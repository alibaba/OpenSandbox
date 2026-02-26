# Copyright 2026 Alibaba Group Holding Ltd.
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#     http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.24-bookworm AS builder

WORKDIR /workspace

# Copy only go mod/sum first for better caching
COPY components/egress/go.mod components/egress/go.sum ./components/egress/
# Bring internal module so replace ../internal works during download/build
COPY components/internal ./components/internal

WORKDIR /workspace/components/egress

# Static-ish build (no cgo) to simplify runtime deps
ENV CGO_ENABLED=0
RUN go mod download

# Copy the rest of the egress sources
COPY components/egress ./ 
RUN go build -o /out/egress .

FROM debian:bookworm-slim

# iptables is needed for DNS REDIRECT; ca-certificates for TLS to upstream resolvers
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        iptables \
        nftables \
        ca-certificates \
        sudo \
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
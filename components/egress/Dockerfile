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

FROM golang:1.25.9-bookworm AS builder

WORKDIR /workspace

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

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
RUN CGO_ENABLED=0 go build \
    -ldflags "-X 'github.com/alibaba/opensandbox/internal/version.Version=${VERSION}' \
              -X 'github.com/alibaba/opensandbox/internal/version.BuildTime=${BUILD_TIME}' \
              -X 'github.com/alibaba/opensandbox/internal/version.GitCommit=${GIT_COMMIT}'" \
    -o /out/egress .

FROM debian:bookworm-slim

# iptables is needed for DNS REDIRECT; ca-certificates for TLS to upstream resolvers
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        iptables \
        iproute2 \
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
        python3 \
        python3-pip \
    && rm -rf /var/lib/apt/lists/*

# Python mitmproxy (transparent mode): mitmdump runs as user mitmproxy; iptables skips this uid.
RUN useradd -r -u 10042 -d /var/lib/mitmproxy -s /usr/sbin/nologin mitmproxy \
    && mkdir -p /var/lib/mitmproxy \
    && chown mitmproxy:mitmproxy /var/lib/mitmproxy \
    && pip3 install --no-cache-dir --break-system-packages 'mitmproxy>=10,<11' \
    && (command -v mitmdump && mitmdump --version) \
    && mkdir -p /var/egress/mitmscripts

COPY --from=builder /out/egress /egress

COPY components/egress/mitmscripts /var/egress/mitmscripts

# Default entrypoint; expects OPENSANDBOX_NETWORK_POLICY env at runtime.
ENTRYPOINT ["/egress"]

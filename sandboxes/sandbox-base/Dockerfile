# SPDX-FileCopyrightText: 2025 Weibo, Inc.
#
# SPDX-License-Identifier: Apache-2.0

# Build context: repo root

# --- Stage 1: Build the HTTP server ---
FROM node:20-slim AS server-builder

RUN npm install -g npm@11

WORKDIR /app

COPY sandboxes/claude-agent-server/package.json sandboxes/claude-agent-server/package-lock.json ./
# npm install is used instead of npm ci to tolerate platform-specific
# transitive dependency resolution differences between build environments.
RUN npm install

COPY sandboxes/claude-agent-server/src/ ./src/
COPY sandboxes/claude-agent-server/docs/ ./docs/
COPY sandboxes/claude-agent-server/tsconfig.json ./
RUN npm run build

# Prune devDependencies so only production deps land in the final image
RUN npm prune --production

# --- Stage 2: Code interpreter base ---
# Mirrors claude-agent-server/code-interpreter/Dockerfile_base with repo-root COPY paths.
FROM ubuntu:24.04 AS code-interpreter-base

# TODO: remove mirrors if not needed
# Configure APT mirrors — Aliyun → TUNA → USTC → upstream (fallback chain).
# Ubuntu 24.04 uses DEB822 format; multiple space-separated URIs are tried in order.
RUN if [ -f /etc/apt/sources.list.d/ubuntu.sources ]; then \
    sed -i \
    -e 's|URIs: http://archive.ubuntu.com/ubuntu|URIs: http://mirrors.aliyun.com/ubuntu http://mirrors.tuna.tsinghua.edu.cn/ubuntu http://mirrors.ustc.edu.cn/ubuntu http://archive.ubuntu.com/ubuntu|g' \
    -e 's|URIs: http://security.ubuntu.com/ubuntu|URIs: http://mirrors.aliyun.com/ubuntu http://mirrors.tuna.tsinghua.edu.cn/ubuntu http://mirrors.ustc.edu.cn/ubuntu http://security.ubuntu.com/ubuntu|g' \
    -e 's|URIs: http://ports.ubuntu.com/ubuntu-ports|URIs: http://mirrors.aliyun.com/ubuntu-ports http://mirrors.tuna.tsinghua.edu.cn/ubuntu-ports http://mirrors.ustc.edu.cn/ubuntu-ports http://ports.ubuntu.com/ubuntu-ports|g' \
    /etc/apt/sources.list.d/ubuntu.sources; \
    fi

ARG TARGETARCH

SHELL ["/bin/bash", "-c"]

ENV DEBIAN_FRONTEND=noninteractive \
    LANG=C.UTF-8 \
    UV_PYTHON_INSTALL_DIR=/opt/python/versions \
    NODE_ROOT=/opt/node \
    NODE_V18=18.20.3 \
    NODE_V20=20.14.0 \
    NODE_V22=22.2.0

# 1. Install common tools
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl wget git vim nano unzip zip tar build-essential \
    gnupg lsb-release \
    && rm -rf /var/lib/apt/lists/*

# 2. Install uv and Python (3.10 - 3.14)
# Pin to a specific version for reproducible builds, e.g. ghcr.io/astral-sh/uv:0.x.y
COPY --from=ghcr.io/astral-sh/uv:0.11.7 /uv /uvx /usr/local/bin/
RUN mkdir -p /opt/python/versions && \
    uv python install 3.10 3.11 3.12 3.13 && \
    (uv python install 3.14 || echo "Python 3.14 skipped")

# 3. Install Node.js (18, 20, 22)
RUN mkdir -p ${NODE_ROOT} && \
    ARCH="" && \
    if [ -z "${TARGETARCH}" ]; then \
    case "$(uname -m)" in \
    x86_64) TARGETARCH="amd64" ;; \
    aarch64) TARGETARCH="arm64" ;; \
    *) echo "Unsupported architecture: $(uname -m)"; exit 1 ;; \
    esac; \
    fi && \
    case "${TARGETARCH}" in \
    "amd64") ARCH="x64" ;; \
    "arm64") ARCH="arm64" ;; \
    *) echo "Unsupported architecture: ${TARGETARCH}"; exit 1 ;; \
    esac && \
    cd ${NODE_ROOT} && \
    for v in ${NODE_V18} ${NODE_V20} ${NODE_V22}; do \
    curl -fsSL https://nodejs.org/dist/v${v}/node-v${v}-linux-${ARCH}.tar.xz | tar -xJ && \
    mv node-v${v}-linux-${ARCH} v${v}; \
    done

# 4. Configure defaults & env script
ENV PATH="${NODE_ROOT}/v${NODE_V22}/bin:${PATH}"

RUN mkdir -p /opt/opensandbox
COPY sandboxes/sandbox-base/code-interpreter-env.sh /opt/opensandbox/code-interpreter-env.sh
RUN chmod +x /opt/opensandbox/code-interpreter-env.sh

# --- Stage 3: Code interpreter ---
FROM code-interpreter-base AS code-interpreter

ARG TARGETARCH

# Install clone3-workaround (amd64 only) for compatibility with older Docker hosts
RUN set -eux; \
    if [ "${TARGETARCH}" = "amd64" ]; then \
    apt-get update && apt-get install -y --no-install-recommends libseccomp2 && rm -rf /var/lib/apt/lists/*; \
    curl -fsSL -o /usr/local/bin/clone3-workaround \
    https://github.com/AkihiroSuda/clone3-workaround/releases/download/v1.0.0/clone3-workaround.x86_64; \
    chmod 755 /usr/local/bin/clone3-workaround; \
    else \
    echo "Skipping clone3-workaround: upstream provides amd64/x86_64 binary only (TARGETARCH=${TARGETARCH})"; \
    fi

ENV PYTHON_VERSION=3.14 \
    NODE_VERSION=22

# --- Stage 4: Final sandbox image (code-interpreter + HTTP server) ---
FROM code-interpreter AS runtime

# ARG ORANGEFS_URL: set to the orangefs binary download URL to include shared-workspace support.
# Leave empty (default) to skip — the entrypoint handles a missing binary gracefully.
ARG ORANGEFS_URL="https://s3-gzpu-inter.didiglobal.com/gift/ofs/bin/orangefs"

RUN apt-get update && apt-get install -y --no-install-recommends fuse && rm -rf /var/lib/apt/lists/*

# Install Claude Code CLI
RUN npm install -g @anthropic-ai/claude-code

RUN if [ -n "${ORANGEFS_URL}" ]; then \
    curl -fsSL "${ORANGEFS_URL}" -o /usr/local/bin/orangefs && \
    chmod +x /usr/local/bin/orangefs; \
    fi

# Copy the compiled server, static assets, and production dependencies from the builder
COPY --from=server-builder /app/dist /app/dist
COPY --from=server-builder /app/docs /app/docs
COPY --from=server-builder /app/node_modules /app/node_modules
COPY --from=server-builder /app/package.json /app/package.json

COPY sandboxes/sandbox-base/bootstrap.sh /opt/opensandbox/bootstrap.sh
COPY sandboxes/sandbox-base/entrypoint.sh /entrypoint.sh
RUN chmod +x /opt/opensandbox/bootstrap.sh /entrypoint.sh

# HTTP server port
EXPOSE 3000

WORKDIR /workspace

# ENTRYPOINT is always overridden to /opt/opensandbox/bootstrap.sh by the opensandbox server.
# CMD is always overridden by request.entrypoint — callers must pass ["/entrypoint.sh"].
ENTRYPOINT ["/opt/opensandbox/bootstrap.sh"]

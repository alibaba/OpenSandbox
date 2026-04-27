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

FROM python:3.10-alpine AS builder

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    UV_PROJECT_ENV=/app/.venv \
    UV_LINK_MODE=copy \
    UV_COMPILE_BYTECODE=1

WORKDIR /app

# Pin to a specific version for reproducible builds, e.g. ghcr.io/astral-sh/uv:0.x.y
COPY --from=ghcr.io/astral-sh/uv:0.11.7 /uv /usr/local/bin/uv

COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev --no-install-project

COPY opensandbox_server ./opensandbox_server
COPY LICENSE README.md ./

# Install the project as a wheel (non-editable) so the runtime stage needs no source copy
RUN uv sync --frozen --no-dev --no-editable

FROM python:3.10-alpine AS runtime

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    UV_PROJECT_ENV=/app/.venv \
    PATH="/app/.venv/bin:${PATH}" \
    SANDBOX_CONFIG_PATH=/etc/opensandbox/config.toml

WORKDIR /app

COPY --from=builder /app/.venv /app/.venv
COPY --from=builder /app/opensandbox_server/examples/example.config.k8s.toml /etc/opensandbox/config.toml
COPY --from=builder /app/opensandbox_server/examples/example.config.k8s.zh.toml /etc/opensandbox/config.zh.toml
COPY --from=builder /app/opensandbox_server/examples/example.batchsandbox-template.yaml /etc/opensandbox/example.batchsandbox-template.yaml
COPY --from=builder /app/opensandbox_server/examples/e2e.batchsandbox-template.yaml /etc/opensandbox/e2e.batchsandbox-template.yaml

EXPOSE 8090

ENTRYPOINT ["opensandbox-server"]
CMD ["--config", "/etc/opensandbox/config.toml"]

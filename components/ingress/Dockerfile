# Copyright 2025 Alibaba Group Holding Ltd.
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

FROM golang:1.24.0 AS builder

WORKDIR /build

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

COPY kubernetes ./kubernetes
# Prepare local modules to satisfy replace directives.
COPY components/internal/go.mod components/internal/go.sum ./components/internal/
COPY components/ingress/go.mod components/ingress/go.sum ./components/ingress/

WORKDIR /build

RUN cd components/internal && go mod download
RUN cd components/ingress && go mod download

# Copy sources.
COPY components/internal ./components/internal
COPY components/ingress/. ./components/ingress

WORKDIR /build/components/ingress

RUN CGO_ENABLED=0 go build \
    -ldflags "-X 'github.com/alibaba/opensandbox/ingress/version.Version=${VERSION}' \
              -X 'github.com/alibaba/opensandbox/ingress/version.BuildTime=${BUILD_TIME}' \
              -X 'github.com/alibaba/opensandbox/ingress/version.GitCommit=${GIT_COMMIT}'" \
    -o /build/ingress ./main.go

FROM alpine:latest

COPY --from=builder /build/ingress .

ENTRYPOINT ["./ingress"]

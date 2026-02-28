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


from src.config import (
    GatewayConfig,
    GatewayRouteModeConfig,
    IngressConfig,
    INGRESS_MODE_DIRECT,
    INGRESS_MODE_GATEWAY,
)
from src.services.constants import OPEN_SANDBOX_INGRESS_HEADER
from src.services.helpers import format_ingress_endpoint


def test_format_ingress_endpoint_returns_none_when_not_gateway():
    cfg = IngressConfig(mode=INGRESS_MODE_DIRECT)
    assert format_ingress_endpoint(cfg, "sid", 8080) is None
    assert format_ingress_endpoint(None, "sid", 8080) is None


def test_format_ingress_endpoint_wildcard():
    cfg = IngressConfig(
        mode=INGRESS_MODE_GATEWAY,
        gateway=GatewayConfig(
            address="*.example.com",
            route=GatewayRouteModeConfig(mode="wildcard"),
        ),
    )
    endpoint = format_ingress_endpoint(cfg, "sid", 8080)
    assert endpoint is not None
    assert endpoint.endpoint == "sid-8080.example.com"
    assert endpoint.headers is None


def test_format_ingress_endpoint_uri():
    cfg = IngressConfig(
        mode=INGRESS_MODE_GATEWAY,
        gateway=GatewayConfig(
            address="gateway.example.com",
            route=GatewayRouteModeConfig(mode="uri"),
        ),
    )
    endpoint = format_ingress_endpoint(cfg, "sid", 9000)
    assert endpoint is not None
    assert endpoint.endpoint == "gateway.example.com/sid/9000"
    assert endpoint.headers is None


def test_format_ingress_endpoint_header():
    cfg = IngressConfig(
        mode=INGRESS_MODE_GATEWAY,
        gateway=GatewayConfig(
            address="gateway.example.com",
            route=GatewayRouteModeConfig(mode="header"),
        ),
    )
    endpoint = format_ingress_endpoint(cfg, "sid", 8080)
    assert endpoint is not None
    assert endpoint.endpoint == "gateway.example.com"
    assert endpoint.headers == {OPEN_SANDBOX_INGRESS_HEADER: "sid-8080"}
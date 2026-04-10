# Copyright 2026 Alibaba Group Holding Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.

"""Tests for opensandbox_server.services.factory (sandbox + pool service creation)."""

from unittest.mock import MagicMock, patch

from opensandbox_server.config import (
    AgentSandboxRuntimeConfig,
    AppConfig,
    KubernetesRuntimeConfig,
    RuntimeConfig,
    ServerConfig,
)
from opensandbox_server.services.factory import create_pool_service
from opensandbox_server.services.k8s.pool_service import PoolService
from opensandbox_server.services.k8s.provider_factory import PROVIDER_TYPE_BATCHSANDBOX


def _server() -> ServerConfig:
    return ServerConfig(host="0.0.0.0", port=8080, log_level="DEBUG", api_key="k")


def _k8s_cfg(**kwargs) -> KubernetesRuntimeConfig:
    base = dict(
        kubeconfig_path="/tmp/test-kubeconfig",
        namespace="ns",
        service_account="sa",
        workload_provider=PROVIDER_TYPE_BATCHSANDBOX,
    )
    base.update(kwargs)
    return KubernetesRuntimeConfig(**base)


def test_create_pool_service_none_for_docker_runtime():
    cfg = AppConfig(
        server=_server(),
        runtime=RuntimeConfig(type="docker", execd_image="img:test"),
        kubernetes=None,
    )
    assert create_pool_service(cfg) is None


def test_create_pool_service_none_for_agent_sandbox():
    k8s = _k8s_cfg(workload_provider="agent-sandbox")
    app = AppConfig(
        server=_server(),
        runtime=RuntimeConfig(type="kubernetes", execd_image="img:test"),
        kubernetes=k8s,
        agent_sandbox=AgentSandboxRuntimeConfig(),
    )
    assert create_pool_service(app) is None


def test_create_pool_service_returns_pool_for_batchsandbox():
    k8s = _k8s_cfg()
    app = AppConfig(
        server=_server(),
        runtime=RuntimeConfig(type="kubernetes", execd_image="img:test"),
        kubernetes=k8s,
    )
    with patch("opensandbox_server.services.factory.K8sClient") as mock_cls:
        mock_cls.return_value = MagicMock()
        svc = create_pool_service(app)
    assert isinstance(svc, PoolService)
    mock_cls.assert_called_once_with(k8s)


def test_create_pool_service_default_workload_provider_is_batchsandbox():
    """Unset workload_provider resolves to default (batchsandbox) -> pool service created."""
    k8s = _k8s_cfg(workload_provider=None)
    app = AppConfig(
        server=_server(),
        runtime=RuntimeConfig(type="kubernetes", execd_image="img:test"),
        kubernetes=k8s,
    )
    with patch("opensandbox_server.services.factory.K8sClient") as mock_cls:
        mock_cls.return_value = MagicMock()
        svc = create_pool_service(app)
    assert isinstance(svc, PoolService)

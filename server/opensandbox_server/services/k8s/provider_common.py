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

from __future__ import annotations

from typing import Any, Callable, Dict, Optional

from kubernetes.client import (
    V1Container,
    V1EnvVar,
    V1ResourceRequirements,
    V1VolumeMount,
)

from opensandbox_server.api.schema import ImageSpec
from opensandbox_server.services.k8s.egress_helper import (
    build_security_context_for_sandbox_container,
    prep_execd_init_for_egress,
)
from opensandbox_server.services.k8s.security_context import (
    build_security_context_from_dict,
    serialize_security_context_to_dict,
)


def _build_execd_init_container(
    execd_image: str,
    execd_init_resources: Any,
    *,
    disable_ipv6_for_egress: bool = False,
) -> V1Container:
    script = (
        "cp ./execd /opt/opensandbox/bin/execd && "
        "cp ./bootstrap.sh /opt/opensandbox/bin/bootstrap.sh && "
        "chmod +x /opt/opensandbox/bin/execd && "
        "chmod +x /opt/opensandbox/bin/bootstrap.sh"
    )
    security_context = None
    if disable_ipv6_for_egress:
        script, sc_dict = prep_execd_init_for_egress(script)
        security_context = build_security_context_from_dict(sc_dict)

    resources = None
    if execd_init_resources:
        resources = V1ResourceRequirements(
            limits=execd_init_resources.limits,
            requests=execd_init_resources.requests,
        )

    return V1Container(
        name="execd-installer",
        image=execd_image,
        command=["/bin/sh", "-c"],
        args=[script],
        volume_mounts=[
            V1VolumeMount(
                name="opensandbox-bin",
                mount_path="/opt/opensandbox/bin",
            )
        ],
        resources=resources,
        security_context=security_context,
    )


def _build_main_container(
    image_spec: ImageSpec,
    entrypoint: list[str],
    env: Dict[str, str],
    resource_limits: Dict[str, str],
    *,
    include_execd_volume: bool,
    has_network_policy: bool = False,
    image_pull_policy: Optional[str] = None,
) -> V1Container:
    env_vars = [V1EnvVar(name=k, value=v) for k, v in env.items()]
    env_vars.append(V1EnvVar(name="EXECD", value="/opt/opensandbox/bin/execd"))

    resources = None
    if resource_limits:
        resources = V1ResourceRequirements(
            limits=resource_limits,
            requests=resource_limits,
        )

    volume_mounts = None
    if include_execd_volume:
        volume_mounts = [
            V1VolumeMount(
                name="opensandbox-bin",
                mount_path="/opt/opensandbox/bin",
            )
        ]

    security_context = None
    if has_network_policy:
        security_context_dict = build_security_context_for_sandbox_container(True)
        security_context = build_security_context_from_dict(security_context_dict)

    return V1Container(
        name="sandbox",
        image=image_spec.uri,
        image_pull_policy=image_pull_policy,
        command=["/opt/opensandbox/bin/bootstrap.sh"] + entrypoint,
        env=env_vars if env_vars else None,
        resources=resources,
        volume_mounts=volume_mounts,
        security_context=security_context,
    )


def _container_to_dict(container: V1Container) -> Dict[str, Any]:
    result: Dict[str, Any] = {
        "name": container.name,
        "image": container.image,
    }
    if container.image_pull_policy:
        result["imagePullPolicy"] = container.image_pull_policy
    if container.command:
        result["command"] = container.command
    if container.args:
        result["args"] = container.args
    if container.env:
        result["env"] = [{"name": e.name, "value": e.value} for e in container.env]
    if container.resources:
        result["resources"] = {}
        if container.resources.limits:
            result["resources"]["limits"] = container.resources.limits
        if container.resources.requests:
            result["resources"]["requests"] = container.resources.requests
    if container.volume_mounts:
        result["volumeMounts"] = [
            {"name": vm.name, "mountPath": vm.mount_path}
            for vm in container.volume_mounts
        ]
    if container.security_context:
        security_context_dict = serialize_security_context_to_dict(container.security_context)
        if security_context_dict:
            result["securityContext"] = security_context_dict
    return result


def _workload_platform_constraint_scope(
    workload: Dict[str, Any],
    pod_template_key: str,
    analyzer: Callable[[Any], tuple[bool, bool]],
) -> tuple[bool, bool]:
    pod_spec = (
        workload.get("spec", {})
        .get(pod_template_key, {})
        .get("spec", {})
    )
    return analyzer(pod_spec)


def _extract_platform_unschedulable_message_from_pod(
    pod: Any,
    workload_has_platform_constraints: bool,
    workload_has_non_platform_constraints: bool,
    checker: Callable[[Optional[str], Optional[str], bool, bool], bool],
) -> Optional[str]:
    if not workload_has_platform_constraints:
        return None
    pod_status = pod.get("status") if isinstance(pod, dict) else getattr(pod, "status", None)
    if pod_status is None:
        return None

    conditions = (
        pod_status.get("conditions", [])
        if isinstance(pod_status, dict)
        else getattr(pod_status, "conditions", []) or []
    )
    for condition in conditions:
        condition_type = (
            condition.get("type")
            if isinstance(condition, dict)
            else getattr(condition, "type", None)
        )
        condition_status = (
            condition.get("status")
            if isinstance(condition, dict)
            else getattr(condition, "status", None)
        )
        condition_reason = (
            condition.get("reason")
            if isinstance(condition, dict)
            else getattr(condition, "reason", None)
        )
        condition_message = (
            condition.get("message")
            if isinstance(condition, dict)
            else getattr(condition, "message", None)
        )
        if (
            condition_type == "PodScheduled"
            and str(condition_status).lower() == "false"
            and checker(
                condition_reason,
                condition_message,
                workload_has_platform_constraints,
                workload_has_non_platform_constraints,
            )
        ):
            return (
                condition_message
                if isinstance(condition_message, str) and condition_message
                else "Pod scheduling constraints cannot be satisfied."
            )

    pod_reason = (
        pod_status.get("reason")
        if isinstance(pod_status, dict)
        else getattr(pod_status, "reason", None)
    )
    pod_message = (
        pod_status.get("message")
        if isinstance(pod_status, dict)
        else getattr(pod_status, "message", None)
    )
    if checker(
        pod_reason,
        pod_message,
        workload_has_platform_constraints,
        workload_has_non_platform_constraints,
    ):
        return (
            pod_message
            if isinstance(pod_message, str) and pod_message
            else "Pod scheduling constraints cannot be satisfied."
        )
    return None

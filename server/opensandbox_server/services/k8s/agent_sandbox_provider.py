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

"""
Agent-sandbox workload provider implementation.
"""

import hashlib
import logging
import re
from datetime import datetime
from typing import Dict, List, Any, Optional

from kubernetes.client import (
    V1Container,
    V1EnvVar,
    V1ResourceRequirements,
    V1VolumeMount,
)

from opensandbox_server.config import AppConfig, DEFAULT_EGRESS_DISABLE_IPV6, EGRESS_MODE_DNS
from opensandbox_server.services.helpers import format_ingress_endpoint
from opensandbox_server.api.schema import Endpoint, ImageSpec, NetworkPolicy, PlatformSpec, Volume
from opensandbox_server.services.k8s.agent_sandbox_template import AgentSandboxTemplateManager
from opensandbox_server.services.k8s.client import K8sClient
from opensandbox_server.services.k8s.egress_helper import (
    apply_egress_to_spec,
    build_security_context_for_sandbox_container,
    prep_execd_init_for_egress,
)
from opensandbox_server.services.k8s.security_context import (
    build_security_context_from_dict,
    serialize_security_context_to_dict,
)
from opensandbox_server.services.k8s.volume_helper import apply_volumes_to_pod_spec
from opensandbox_server.services.k8s.workload_provider import WorkloadProvider
from opensandbox_server.services.runtime_resolver import SecureRuntimeResolver

logger = logging.getLogger(__name__)

DNS1035_LABEL_MAX_LENGTH = 63
DNS1035_INVALID_CHARS = re.compile(r"[^a-z0-9-]+")
DNS1035_DUPLICATE_HYPHENS = re.compile(r"-+")


def _to_dns1035_label(value: str, prefix: str = "sandbox") -> str:
    normalized = DNS1035_INVALID_CHARS.sub("-", value.strip().lower())
    normalized = DNS1035_DUPLICATE_HYPHENS.sub("-", normalized).strip("-")

    hash_suffix = hashlib.sha256(value.encode("utf-8")).hexdigest()[:8]

    if not normalized:
        normalized = f"{prefix}-{hash_suffix}"
    elif not normalized[0].isalpha():
        normalized = f"{prefix}-{normalized}"

    if len(normalized) > DNS1035_LABEL_MAX_LENGTH:
        max_base = DNS1035_LABEL_MAX_LENGTH - len(hash_suffix) - 1
        base = normalized[:max_base].rstrip("-")
        if not base or not base[0].isalpha():
            base = prefix
        normalized = f"{base}-{hash_suffix}"

    return normalized.strip("-")


class AgentSandboxProvider(WorkloadProvider):
    """
    Workload provider using kubernetes-sigs/agent-sandbox Sandbox CRD.
    """

    def __init__(
        self,
        k8s_client: K8sClient,
        app_config: Optional[AppConfig] = None,
    ):
        self.k8s_client = k8s_client

        self.group = "agents.x-k8s.io"
        self.version = "v1alpha1"
        self.plural = "sandboxes"

        k8s_config = app_config.kubernetes if app_config else None
        agent_config = app_config.agent_sandbox if app_config else None

        self.shutdown_policy = agent_config.shutdown_policy if agent_config else "Delete"
        self.service_account = k8s_config.service_account if k8s_config else None
        self.template_manager = AgentSandboxTemplateManager(
            agent_config.template_file if agent_config else None
        )
        self.ingress_config = app_config.ingress if app_config else None
        self.execd_init_resources = k8s_config.execd_init_resources if k8s_config else None

        # Initialize secure runtime resolver
        self.resolver = SecureRuntimeResolver(app_config) if app_config else None
        self.runtime_class = (
            self.resolver.get_k8s_runtime_class() if self.resolver else None
        )

        self.egress_disable_ipv6 = (
            bool(app_config.egress.disable_ipv6)
            if app_config and app_config.egress is not None
            else DEFAULT_EGRESS_DISABLE_IPV6
        )

    def _resource_name(self, sandbox_id: str) -> str:
        return _to_dns1035_label(sandbox_id, prefix="sandbox")

    def _resource_name_candidates(self, sandbox_id: str) -> List[str]:
        candidates = []
        primary = self._resource_name(sandbox_id)
        candidates.append(primary)
        if sandbox_id not in candidates:
            candidates.append(sandbox_id)
        legacy = self.legacy_resource_name(sandbox_id)
        if legacy not in candidates:
            candidates.append(legacy)
        return candidates

    def create_workload(
        self,
        sandbox_id: str,
        namespace: str,
        image_spec: ImageSpec,
        entrypoint: List[str],
        env: Dict[str, str],
        resource_limits: Dict[str, str],
        labels: Dict[str, str],
        expires_at: Optional[datetime],
        execd_image: str,
        extensions: Optional[Dict[str, str]] = None,
        network_policy: Optional[NetworkPolicy] = None,
        egress_image: Optional[str] = None,
        volumes: Optional[List[Volume]] = None,
        platform: Optional[PlatformSpec] = None,
        annotations: Optional[Dict[str, str]] = None,
        egress_auth_token: Optional[str] = None,
        egress_mode: str = EGRESS_MODE_DNS,
        resource_requests: Optional[Dict[str, str]] = None,
    ) -> Dict[str, Any]:
        """Create an agent-sandbox Sandbox CRD workload."""
        if self.runtime_class:
            logger.info(
                "Using Kubernetes RuntimeClass '%s' for sandbox %s",
                self.runtime_class,
                sandbox_id,
            )

        pod_spec = self._build_pod_spec(
            image_spec=image_spec,
            entrypoint=entrypoint,
            env=env,
            resource_limits=resource_limits,
            execd_image=execd_image,
            network_policy=network_policy,
            egress_image=egress_image,
            egress_auth_token=egress_auth_token,
            egress_mode=egress_mode,
            resource_requests=resource_requests,
        )

        # Add user-specified volumes if provided
        if volumes:
            apply_volumes_to_pod_spec(pod_spec, volumes)

        if self.service_account:
            pod_spec["serviceAccountName"] = self.service_account
        self._apply_platform_node_selector(pod_spec, platform)

        resource_name = self._resource_name(sandbox_id)
        spec = {
            "replicas": 1,
            "shutdownPolicy": self.shutdown_policy,
            "podTemplate": {
                "metadata": {
                    "labels": labels,
                },
                "spec": pod_spec,
            },
        }
        runtime_manifest = {
            "apiVersion": f"{self.group}/{self.version}",
            "kind": "Sandbox",
            "metadata": {
                "name": resource_name,
                "namespace": namespace,
                "labels": labels,
            },
            "spec": spec,
        }
        if annotations:
            runtime_manifest["metadata"]["annotations"] = annotations

        sandbox = self.template_manager.merge_with_runtime_values(runtime_manifest)
        # Set or strip shutdownTime after merge so we override any template value
        if expires_at is None:
            sandbox["spec"].pop("shutdownTime", None)
        else:
            sandbox["spec"]["shutdownTime"] = expires_at.isoformat()
        if platform is not None:
            merged_pod_spec = sandbox.get("spec", {}).get("podTemplate", {}).get("spec", {})
            WorkloadProvider.ensure_platform_compatible_with_affinity(merged_pod_spec, platform)

        created = self.k8s_client.create_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            body=sandbox,
        )

        return {
            "name": created["metadata"]["name"],
            "uid": created["metadata"]["uid"],
        }

    def _apply_platform_node_selector(
        self,
        pod_spec: Dict[str, Any],
        platform: Optional[PlatformSpec],
    ) -> None:
        if platform is None:
            return

        template = self.template_manager.get_base_template()
        template_spec = (
            template.get("spec", {})
            .get("podTemplate", {})
            .get("spec", {})
        )
        WorkloadProvider.apply_platform_node_selector(
            pod_spec=pod_spec,
            template_spec=template_spec if isinstance(template_spec, dict) else {},
            platform=platform,
        )

    def _build_pod_spec(
        self,
        image_spec: ImageSpec,
        entrypoint: List[str],
        env: Dict[str, str],
        resource_limits: Dict[str, str],
        execd_image: str,
        network_policy: Optional[NetworkPolicy] = None,
        egress_image: Optional[str] = None,
        egress_auth_token: Optional[str] = None,
        egress_mode: str = EGRESS_MODE_DNS,
        resource_requests: Optional[Dict[str, str]] = None,
    ) -> Dict[str, Any]:
        """Build pod spec dict for the Sandbox CRD."""
        disable_ipv6_for_egress = (
            network_policy is not None
            and egress_image is not None
            and self.egress_disable_ipv6
        )
        init_container = self._build_execd_init_container(
            execd_image, disable_ipv6_for_egress=disable_ipv6_for_egress
        )
        main_container = self._build_main_container(
            image_spec=image_spec,
            entrypoint=entrypoint,
            env=env,
            resource_limits=resource_limits,
            include_execd_volume=True,
            has_network_policy=network_policy is not None,
            resource_requests=resource_requests,
        )
        
        containers = [self._container_to_dict(main_container)]
        
        # Build base pod spec
        pod_spec: Dict[str, Any] = {
            "initContainers": [self._container_to_dict(init_container)],
            "containers": containers,
            "volumes": [
                {
                    "name": "opensandbox-bin",
                    "emptyDir": {},
                }
            ],
        }

        # Inject runtimeClassName if secure runtime is configured
        if self.runtime_class:
            pod_spec["runtimeClassName"] = self.runtime_class

        # Add egress sidecar if network policy is provided
        apply_egress_to_spec(
            containers=containers,
            network_policy=network_policy,
            egress_image=egress_image,
            egress_auth_token=egress_auth_token,
            egress_mode=egress_mode,
        )

        return pod_spec

    def _build_execd_init_container(
        self,
        execd_image: str,
        *,
        disable_ipv6_for_egress: bool = False,
    ) -> V1Container:
        """Build init container that copies execd binary to the shared volume.

        ``disable_ipv6_for_egress`` is True only when ``egress.disable_ipv6`` is set and egress is used.
        """
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
        if self.execd_init_resources:
            resources = V1ResourceRequirements(
                limits=self.execd_init_resources.limits,
                requests=self.execd_init_resources.requests,
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
        self,
        image_spec: ImageSpec,
        entrypoint: List[str],
        env: Dict[str, str],
        resource_limits: Dict[str, str],
        include_execd_volume: bool,
        has_network_policy: bool = False,
        resource_requests: Optional[Dict[str, str]] = None,
    ) -> V1Container:
        env_vars = [V1EnvVar(name=k, value=v) for k, v in env.items()]
        env_vars.append(V1EnvVar(name="EXECD", value="/opt/opensandbox/bin/execd"))

        resources = None
        if resource_limits:
            requests = resource_requests if resource_requests else resource_limits
            resources = V1ResourceRequirements(
                limits=resource_limits,
                requests=requests,
            )

        wrapped_command = ["/opt/opensandbox/bin/bootstrap.sh"] + entrypoint

        volume_mounts = None
        if include_execd_volume:
            volume_mounts = [
                V1VolumeMount(
                    name="opensandbox-bin",
                    mount_path="/opt/opensandbox/bin",
                )
            ]

        # Apply security context when network policy is enabled
        security_context = None
        if has_network_policy:
            security_context_dict = build_security_context_for_sandbox_container(True)
            security_context = build_security_context_from_dict(security_context_dict)

        return V1Container(
            name="sandbox",
            image=image_spec.uri,
            command=wrapped_command,
            env=env_vars if env_vars else None,
            resources=resources,
            volume_mounts=volume_mounts,
            security_context=security_context,
        )

    def _container_to_dict(self, container: V1Container) -> Dict[str, Any]:
        """Convert a V1Container object to a plain dict for CRD body."""
        result: Dict[str, Any] = {
            "name": container.name,
            "image": container.image,
        }

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

    def get_workload(self, sandbox_id: str, namespace: str) -> Optional[Dict[str, Any]]:
        """Get Sandbox CRD by sandbox ID, trying all candidate resource names."""
        candidates = self._resource_name_candidates(sandbox_id)

        for name in candidates:
            workload = self.k8s_client.get_custom_object(
                group=self.group,
                version=self.version,
                namespace=namespace,
                plural=self.plural,
                name=name,
            )
            if workload:
                return workload

        return None

    def delete_workload(self, sandbox_id: str, namespace: str) -> None:
        """Delete the Sandbox CRD for the given sandbox ID."""
        sandbox = self.get_workload(sandbox_id, namespace)
        if not sandbox:
            raise Exception(f"Sandbox for sandbox {sandbox_id} not found")

        self.k8s_client.delete_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            name=sandbox["metadata"]["name"],
            grace_period_seconds=0,
        )

    def list_workloads(self, namespace: str, label_selector: str) -> List[Dict[str, Any]]:
        """List Sandbox CRDs matching the given label selector."""
        return self.k8s_client.list_custom_objects(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            label_selector=label_selector,
        )

    def update_expiration(self, sandbox_id: str, namespace: str, expires_at: datetime) -> None:
        """Patch the Sandbox CRD shutdownTime field."""
        sandbox = self.get_workload(sandbox_id, namespace)
        if not sandbox:
            raise Exception(f"Sandbox for sandbox {sandbox_id} not found")

        body = {
            "spec": {
                "shutdownTime": expires_at.isoformat(),
            }
        }

        self.k8s_client.patch_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            name=sandbox["metadata"]["name"],
            body=body,
        )

    def get_expiration(self, workload: Dict[str, Any]) -> Optional[datetime]:
        """Parse shutdownTime from Sandbox CRD spec."""
        spec = workload.get("spec", {})
        shutdown_time_str = spec.get("shutdownTime")

        if not shutdown_time_str:
            return None

        try:
            return datetime.fromisoformat(shutdown_time_str.replace("Z", "+00:00"))
        except (ValueError, TypeError) as e:
            logger.warning("Invalid shutdownTime format: %s, error: %s", shutdown_time_str, e)
            return None

    def get_status(self, workload: Dict[str, Any]) -> Dict[str, Any]:
        """Derive sandbox state from the Sandbox CRD status conditions."""
        status = workload.get("status", {})
        conditions = status.get("conditions", [])

        ready_condition = None
        for condition in conditions:
            if condition.get("type") == "Ready":
                ready_condition = condition
                break

        creation_timestamp = workload.get("metadata", {}).get("creationTimestamp")

        if not ready_condition:
            pod_state = self._pod_state_from_selector(workload)
            if pod_state:
                state, reason, message = pod_state
                return {
                    "state": state,
                    "reason": reason,
                    "message": message,
                    "last_transition_at": creation_timestamp,
                }
            return {
                "state": "Pending",
                "reason": "SANDBOX_PENDING",
                "message": "Sandbox is pending scheduling",
                "last_transition_at": creation_timestamp,
            }

        cond_status = ready_condition.get("status")
        reason = ready_condition.get("reason")
        message = ready_condition.get("message")
        last_transition_at = ready_condition.get("lastTransitionTime") or creation_timestamp
        has_platform_constraints, has_non_platform_constraints = (
            self._workload_platform_constraint_scope(workload)
        )

        if cond_status == "True":
            state = "Running"
        elif reason == "SandboxExpired":
            state = "Terminated"
        elif cond_status == "False" and self.is_platform_unschedulable(
            reason,
            message,
            has_platform_constraints,
            has_non_platform_constraints,
        ):
            state = "Failed"
            reason = "POD_PLATFORM_UNSCHEDULABLE"
            message = message or "Pod scheduling constraints cannot be satisfied."
        elif cond_status == "False":
            state = "Pending"
        else:
            state = "Pending"

        return {
            "state": state,
            "reason": reason,
            "message": message,
            "last_transition_at": last_transition_at,
        }

    def _pod_state_from_selector(self, workload: Dict[str, Any]) -> Optional[tuple[str, str, str]]:
        """Resolve state from Pod list via label selector.

        Returns three-state tuple (state, reason, message):
        - Running: Pod phase Running and has IP
        - Allocated: Pod has IP assigned but not Running yet
        - Pending: Pod scheduled but no IP yet
        Returns None if selector/namespace missing or API call fails.
        """
        status = workload.get("status", {})
        selector = status.get("selector")
        namespace = workload.get("metadata", {}).get("namespace")
        if not selector or not namespace:
            return None

        try:
            pods = self.k8s_client.list_pods(
                namespace=namespace,
                label_selector=selector,
            )
        except Exception:
            return None

        has_platform_constraints, has_non_platform_constraints = (
            self._workload_platform_constraint_scope(workload)
        )
        for pod in pods:
            unschedulable_message = self._extract_platform_unschedulable_message_from_pod(
                pod,
                has_platform_constraints,
                has_non_platform_constraints,
            )
            if unschedulable_message:
                return ("Failed", "POD_PLATFORM_UNSCHEDULABLE", unschedulable_message)

            pod_status = pod.get("status") if isinstance(pod, dict) else getattr(pod, "status", None)
            if pod_status:
                pod_ip = (
                    pod_status.get("podIP")
                    if isinstance(pod_status, dict)
                    else getattr(pod_status, "pod_ip", None)
                )
                pod_phase = (
                    pod_status.get("phase")
                    if isinstance(pod_status, dict)
                    else getattr(pod_status, "phase", None)
                )
                if pod_ip and pod_phase == "Running":
                    return (
                        "Running",
                        "POD_READY",
                        "Pod is running with IP assigned",
                    )
                if pod_ip:
                    return (
                        "Allocated",
                        "IP_ASSIGNED",
                        "Pod has IP assigned but not running yet",
                    )
                return (
                    "Pending",
                    "POD_SCHEDULED",
                    "Pod is scheduled but waiting for IP assignment",
                )

        if pods:
            return ("Pending", "POD_PENDING", "Pod is pending")

        return None

    @staticmethod
    def _workload_has_platform_constraints(workload: Dict[str, Any]) -> bool:
        has_platform_constraints, _ = AgentSandboxProvider._workload_platform_constraint_scope(workload)
        return has_platform_constraints

    @staticmethod
    def _workload_platform_constraint_scope(workload: Dict[str, Any]) -> tuple[bool, bool]:
        pod_spec = (
            workload.get("spec", {})
            .get("podTemplate", {})
            .get("spec", {})
        )
        return AgentSandboxProvider.analyze_platform_constraints_in_pod_spec(pod_spec)

    def _extract_platform_unschedulable_message_from_pod(
        self,
        pod: Any,
        workload_has_platform_constraints: bool,
        workload_has_non_platform_constraints: bool,
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
                and self.is_platform_unschedulable(
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
        if self.is_platform_unschedulable(
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

    def get_endpoint_info(self, workload: Dict[str, Any], port: int, sandbox_id: str) -> Optional[Endpoint]:
        # ingress-based endpoint if configured (gateway)
        ingress_endpoint = format_ingress_endpoint(self.ingress_config, sandbox_id, port)
        if ingress_endpoint:
            return ingress_endpoint

        status = workload.get("status", {})
        selector = status.get("selector")
        namespace = workload.get("metadata", {}).get("namespace")
        if selector and namespace:
            try:
                pods = self.k8s_client.list_pods(
                    namespace=namespace,
                    label_selector=selector,
                )
                for pod in pods:
                    if pod.status and pod.status.pod_ip and pod.status.phase == "Running":
                        return Endpoint(endpoint=f"{pod.status.pod_ip}:{port}")
            except Exception as e:
                logger.warning("Failed to resolve pod endpoint: %s", e)

        service_fqdn = status.get("serviceFQDN")
        if service_fqdn:
            return Endpoint(endpoint=f"{service_fqdn}:{port}")

        return None

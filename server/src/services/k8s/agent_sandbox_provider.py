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
from typing import Dict, List, Any, Optional, Callable
from threading import Lock

from kubernetes.client import (
    V1Container,
    V1EnvVar,
    V1ResourceRequirements,
    V1VolumeMount,
    ApiException,
)

from src.config import AppConfig, IngressConfig, ExecdInitResources
from src.services.helpers import format_ingress_endpoint
from src.api.schema import Endpoint, ImageSpec, NetworkPolicy, Volume
from src.services.k8s.agent_sandbox_template import AgentSandboxTemplateManager
from src.services.k8s.client import K8sClient
from src.services.k8s.egress_helper import (
    apply_egress_to_spec,
    build_security_context_for_sandbox_container,
    build_security_context_from_dict,
    serialize_security_context_to_dict,
)
from src.services.k8s.informer import WorkloadInformer
from src.services.k8s.volume_helper import apply_volumes_to_pod_spec
from src.services.k8s.workload_provider import WorkloadProvider
from src.services.runtime_resolver import SecureRuntimeResolver

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
        template_file_path: Optional[str] = None,
        shutdown_policy: str = "Delete",
        service_account: Optional[str] = None,
        ingress_config: Optional[IngressConfig] = None,
        enable_informer: bool = True,
        informer_factory: Optional[Callable[[str], WorkloadInformer]] = None,
        informer_resync_seconds: int = 300,
        informer_watch_timeout_seconds: int = 60,
        app_config: Optional[AppConfig] = None,
        execd_init_resources: Optional[ExecdInitResources] = None,
    ):
        self.k8s_client = k8s_client
        self.custom_api = k8s_client.get_custom_objects_api()
        self.core_api = k8s_client.get_core_v1_api()

        self.group = "agents.x-k8s.io"
        self.version = "v1alpha1"
        self.plural = "sandboxes"

        self.shutdown_policy = shutdown_policy
        self.service_account = service_account
        self.template_manager = AgentSandboxTemplateManager(template_file_path)
        self.ingress_config = ingress_config
        self.execd_init_resources = execd_init_resources
        self._enable_informer = enable_informer
        self._informer_factory = informer_factory or (
            lambda ns: WorkloadInformer(
                custom_api=self.custom_api,
                group=self.group,
                version=self.version,
                plural=self.plural,
                namespace=ns,
                resync_period_seconds=informer_resync_seconds,
                watch_timeout_seconds=informer_watch_timeout_seconds,
            )
        )
        self._informers: Dict[str, WorkloadInformer] = {}
        self._informers_lock = Lock()

        # Initialize secure runtime resolver
        self.resolver = SecureRuntimeResolver(app_config) if app_config else None
        self.runtime_class = (
            self.resolver.get_k8s_runtime_class() if self.resolver else None
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
        expires_at: datetime,
        execd_image: str,
        extensions: Optional[Dict[str, str]] = None,
        network_policy: Optional[NetworkPolicy] = None,
        egress_image: Optional[str] = None,
        volumes: Optional[List[Volume]] = None,
    ) -> Dict[str, Any]:
        # Pool mode does not support volumes
        if extensions and extensions.get("poolRef"):
            if volumes:
                raise ValueError(
                    "Pool mode does not support volumes. "
                    "Remove 'volumes' from request or use template mode."
                )

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
        )

        # Add user-specified volumes if provided
        if volumes:
            apply_volumes_to_pod_spec(pod_spec, volumes)

        if self.service_account:
            pod_spec["serviceAccountName"] = self.service_account

        resource_name = self._resource_name(sandbox_id)

        runtime_manifest = {
            "apiVersion": f"{self.group}/{self.version}",
            "kind": "Sandbox",
            "metadata": {
                "name": resource_name,
                "namespace": namespace,
                "labels": labels,
            },
            "spec": {
                "replicas": 1,
                "shutdownTime": expires_at.isoformat(),
                "shutdownPolicy": self.shutdown_policy,
                "podTemplate": {
                    "metadata": {
                        "labels": labels,
                    },
                    "spec": pod_spec,
                },
            },
        }

        sandbox = self.template_manager.merge_with_runtime_values(runtime_manifest)

        created = self.custom_api.create_namespaced_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            body=sandbox,
        )

        informer = self._get_informer(namespace)
        if informer:
            try:
                informer.update_cache(created)
            except Exception as exc:  # pragma: no cover - defensive
                logger.warning("Failed to update informer cache for %s: %s", sandbox_id, exc)

        return {
            "name": created["metadata"]["name"],
            "uid": created["metadata"]["uid"],
        }

    def _build_pod_spec(
        self,
        image_spec: ImageSpec,
        entrypoint: List[str],
        env: Dict[str, str],
        resource_limits: Dict[str, str],
        execd_image: str,
        network_policy: Optional[NetworkPolicy] = None,
        egress_image: Optional[str] = None,
    ) -> Dict[str, Any]:
        init_container = self._build_execd_init_container(execd_image)
        main_container = self._build_main_container(
            image_spec=image_spec,
            entrypoint=entrypoint,
            env=env,
            resource_limits=resource_limits,
            include_execd_volume=True,
            has_network_policy=network_policy is not None,
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
            pod_spec=pod_spec,
            containers=containers,
            network_policy=network_policy,
            egress_image=egress_image,
        )
        
        return pod_spec

    def _build_execd_init_container(self, execd_image: str) -> V1Container:
        script = (
            "cp ./execd /opt/opensandbox/bin/execd && "
            "cp ./bootstrap.sh /opt/opensandbox/bin/bootstrap.sh && "
            "chmod +x /opt/opensandbox/bin/execd && "
            "chmod +x /opt/opensandbox/bin/bootstrap.sh"
        )

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
        )

    def _build_main_container(
        self,
        image_spec: ImageSpec,
        entrypoint: List[str],
        env: Dict[str, str],
        resource_limits: Dict[str, str],
        include_execd_volume: bool,
        has_network_policy: bool = False,
    ) -> V1Container:
        env_vars = [V1EnvVar(name=k, value=v) for k, v in env.items()]
        env_vars.append(V1EnvVar(name="EXECD", value="/opt/opensandbox/bin/execd"))

        resources = None
        if resource_limits:
            resources = V1ResourceRequirements(
                limits=resource_limits,
                requests=resource_limits,
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

    def _get_informer(self, namespace: str) -> Optional[WorkloadInformer]:
        if not self._enable_informer:
            return None

        with self._informers_lock:
            informer = self._informers.get(namespace)
            if informer is None:
                informer = self._informer_factory(namespace)
                self._informers[namespace] = informer
                try:
                    informer.start()
                except Exception as exc:  # pragma: no cover - defensive
                    logger.warning(
                        "Failed to start informer for namespace %s: %s", namespace, exc
                    )
                    self._informers.pop(namespace, None)
                    return None
        return informer

    def get_workload(self, sandbox_id: str, namespace: str) -> Optional[Dict[str, Any]]:
        informer = self._get_informer(namespace)
        cache_ready = informer.has_synced if informer else False

        candidates = self._resource_name_candidates(sandbox_id)

        if informer and cache_ready:
            for name in candidates:
                cached = informer.get(name)
                if cached:
                    return cached

        if informer and not cache_ready:
            logger.warning(
                f"Informer cache not synced for namespace {namespace}; falling back to direct API get."
            )

        for name in candidates:
            try:
                workload = self.custom_api.get_namespaced_custom_object(
                    group=self.group,
                    version=self.version,
                    namespace=namespace,
                    plural=self.plural,
                    name=name,
                )
                if informer and workload:
                    informer.update_cache(workload)
                return workload
            except ApiException as e:
                if e.status != 404:
                    logger.error(f"Unexpected error getting Sandbox for {sandbox_id}: {e}")
                    raise
            except Exception as e:
                logger.error(f"Unexpected error getting Sandbox for {sandbox_id}: {e}")
                raise

        return None

    def delete_workload(self, sandbox_id: str, namespace: str) -> None:
        sandbox = self.get_workload(sandbox_id, namespace)
        if not sandbox:
            raise Exception(f"Sandbox for sandbox {sandbox_id} not found")

        self.custom_api.delete_namespaced_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            name=sandbox["metadata"]["name"],
            grace_period_seconds=0,
        )

    def list_workloads(self, namespace: str, label_selector: str) -> List[Dict[str, Any]]:
        try:
            sandbox_list = self.custom_api.list_namespaced_custom_object(
                group=self.group,
                version=self.version,
                namespace=namespace,
                plural=self.plural,
                label_selector=label_selector,
            )
            return sandbox_list.get("items", [])
        except ApiException as e:
            if e.status == 404:
                return []
            raise
        except Exception as e:
            logger.error(f"Unexpected error listing Sandboxes: {e}")
            raise

    def update_expiration(self, sandbox_id: str, namespace: str, expires_at: datetime) -> None:
        sandbox = self.get_workload(sandbox_id, namespace)
        if not sandbox:
            raise Exception(f"Sandbox for sandbox {sandbox_id} not found")

        body = {
            "spec": {
                "shutdownTime": expires_at.isoformat(),
            }
        }

        self.custom_api.patch_namespaced_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            name=sandbox["metadata"]["name"],
            body=body,
        )

    def get_expiration(self, workload: Dict[str, Any]) -> Optional[datetime]:
        spec = workload.get("spec", {})
        shutdown_time_str = spec.get("shutdownTime")

        if not shutdown_time_str:
            return None

        try:
            return datetime.fromisoformat(shutdown_time_str.replace("Z", "+00:00"))
        except (ValueError, TypeError) as e:
            logger.warning(f"Invalid shutdownTime format: {shutdown_time_str}, error: {e}")
            return None

    def get_status(self, workload: Dict[str, Any]) -> Dict[str, Any]:
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

        if cond_status == "True":
            state = "Running"
        elif reason == "SandboxExpired":
            state = "Terminated"
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
            pods = self.core_api.list_namespaced_pod(
                namespace=namespace,
                label_selector=selector,
            ).items
        except Exception:
            return None

        for pod in pods:
            if pod.status:
                if pod.status.pod_ip and pod.status.phase == "Running":
                    return (
                        "Running",
                        "POD_READY",
                        "Pod is running with IP assigned",
                    )
                if pod.status.pod_ip:
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
                pods = self.core_api.list_namespaced_pod(
                    namespace=namespace,
                    label_selector=selector,
                ).items
                for pod in pods:
                    if pod.status and pod.status.pod_ip and pod.status.phase == "Running":
                        return Endpoint(endpoint=f"{pod.status.pod_ip}:{port}")
            except Exception as e:
                logger.warning(f"Failed to resolve pod endpoint: {e}")

        service_fqdn = status.get("serviceFQDN")
        if service_fqdn:
            return Endpoint(endpoint=f"{service_fqdn}:{port}")

        return None


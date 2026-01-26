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

import logging
from datetime import datetime
from typing import Dict, List, Any, Optional

from kubernetes.client import (
    V1Container,
    V1EnvVar,
    V1ResourceRequirements,
    V1VolumeMount,
    ApiException,
)

from src.api.schema import ImageSpec
from src.services.constants import SANDBOX_ID_LABEL
from src.services.k8s.agent_sandbox_template import AgentSandboxTemplateManager
from src.services.k8s.client import K8sClient
from src.services.k8s.workload_provider import WorkloadProvider

logger = logging.getLogger(__name__)


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
    ) -> Dict[str, Any]:
        sandbox_name = f"sandbox-{sandbox_id}"

        pod_spec = self._build_pod_spec(
            image_spec=image_spec,
            entrypoint=entrypoint,
            env=env,
            resource_limits=resource_limits,
            execd_image=execd_image,
        )

        if self.service_account:
            pod_spec["serviceAccountName"] = self.service_account

        runtime_manifest = {
            "apiVersion": f"{self.group}/{self.version}",
            "kind": "Sandbox",
            "metadata": {
                "name": sandbox_name,
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
    ) -> Dict[str, Any]:
        init_container = self._build_execd_init_container(execd_image)
        main_container = self._build_main_container(
            image_spec=image_spec,
            entrypoint=entrypoint,
            env=env,
            resource_limits=resource_limits,
            include_execd_volume=True,
        )
        return {
            "initContainers": [self._container_to_dict(init_container)],
            "containers": [self._container_to_dict(main_container)],
            "volumes": [
                {
                    "name": "opensandbox-bin",
                    "emptyDir": {},
                }
            ],
        }

    def _build_execd_init_container(self, execd_image: str) -> V1Container:
        script = (
            "cp ./execd /opt/opensandbox/bin/execd && "
            "cp ./bootstrap.sh /opt/opensandbox/bin/bootstrap.sh && "
            "chmod +x /opt/opensandbox/bin/execd && "
            "chmod +x /opt/opensandbox/bin/bootstrap.sh"
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
        )

    def _build_main_container(
        self,
        image_spec: ImageSpec,
        entrypoint: List[str],
        env: Dict[str, str],
        resource_limits: Dict[str, str],
        include_execd_volume: bool,
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

        return V1Container(
            name="sandbox",
            image=image_spec.uri,
            command=wrapped_command,
            env=env_vars if env_vars else None,
            resources=resources,
            volume_mounts=volume_mounts,
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

        return result

    def get_workload(self, sandbox_id: str, namespace: str) -> Optional[Dict[str, Any]]:
        label_selector = f"{SANDBOX_ID_LABEL}={sandbox_id}"
        try:
            sandbox_list = self.custom_api.list_namespaced_custom_object(
                group=self.group,
                version=self.version,
                namespace=namespace,
                plural=self.plural,
                label_selector=label_selector,
            )
            if sandbox_list.get("items"):
                return sandbox_list["items"][0]
            return None
        except ApiException as e:
            if e.status == 404:
                return None
            raise
        except Exception as e:
            logger.error("Unexpected error getting Sandbox for %s: %s", sandbox_id, e)
            raise

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
            logger.error("Unexpected error listing Sandboxes: %s", e)
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
            logger.warning("Invalid shutdownTime format: %s, error: %s", shutdown_time_str, e)
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
            if pod.status and pod.status.phase == "Running":
                if pod.status.pod_ip:
                    return (
                        "Running",
                        "POD_READY",
                        "Pod is running with IP assigned",
                    )
                return (
                    "Pending",
                    "POD_READY_NO_IP",
                    "Pod is running but waiting for IP assignment",
                )

        if pods:
            return ("Pending", "POD_PENDING", "Pod is pending")

        return None

    def get_endpoint_info(self, workload: Dict[str, Any], port: int) -> Optional[str]:
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
                        return f"{pod.status.pod_ip}:{port}"
            except Exception as e:
                logger.warning("Failed to resolve pod endpoint: %s", e)

        service_fqdn = status.get("serviceFQDN")
        if service_fqdn:
            return f"{service_fqdn}:{port}"

        return None

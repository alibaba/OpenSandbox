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
BatchSandbox-based workload provider implementation.
"""

import logging
import shlex
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
from src.services.k8s.batchsandbox_template import BatchSandboxTemplateManager
from src.services.k8s.client import K8sClient
from src.services.k8s.workload_provider import WorkloadProvider

logger = logging.getLogger(__name__)


class BatchSandboxProvider(WorkloadProvider):
    """
    Workload provider using BatchSandbox CRD.
    
    BatchSandbox is a custom resource that manages Pod lifecycle
    and provides additional features like task management.
    """
    
    def __init__(self, k8s_client: K8sClient, template_file_path: Optional[str] = None):
        """
        Initialize BatchSandbox provider.
        
        Args:
            k8s_client: Kubernetes client wrapper
            template_file_path: Optional path to BatchSandbox CR YAML template file
        """
        self.k8s_client = k8s_client
        self.custom_api = k8s_client.get_custom_objects_api()
        
        # CRD constants
        self.group = "sandbox.opensandbox.io"
        self.version = "v1alpha1"
        self.plural = "batchsandboxes"
        
        # Template manager
        self.template_manager = BatchSandboxTemplateManager(template_file_path)
    
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
        """
        Create a BatchSandbox workload.
        
        Supports both template-based and pool-based creation:
        - Template mode (default): Creates workload with user-specified image, resources, and env
        - Pool mode (when extensions contains 'poolRef'): Creates workload from pre-warmed pool,
          only entrypoint and env can be customized
        
        Args:
            sandbox_id: Unique sandbox identifier
            namespace: Kubernetes namespace
            image_spec: Container image specification (not used in pool mode)
            entrypoint: Container entrypoint command
            env: Environment variables
            resource_limits: Resource limits (not used in pool mode)
            labels: Labels to apply
            expires_at: Expiration time
            execd_image: execd daemon image (not used in pool mode)
            extensions: General extension field for additional configuration.
                When contains 'poolRef', enables pool-based creation.
        
        Returns:
            Dict with 'name' and 'uid' of created BatchSandbox
        """
        batchsandbox_name = f"sandbox-{sandbox_id}"
        extensions = extensions or {}
        
        # If poolRef is provided and not empty, create workload from pool
        if extensions.get("poolRef"):
            # When using pool, only entrypoint and env can be customized
            return self._create_workload_from_pool(
                batchsandbox_name=batchsandbox_name,
                namespace=namespace,
                labels=labels,
                pool_ref=extensions["poolRef"],
                expires_at=expires_at,
                entrypoint=entrypoint,
                env=env,
            )
        
        # Extract extra pod spec fragments from template (volumes/volumeMounts only).
        extra_volumes, extra_mounts = self._extract_template_pod_extras()

        # Build init container for execd installation
        init_container = self._build_execd_init_container(execd_image)
        
        # Build main container with execd support
        main_container = self._build_main_container(
            image_spec=image_spec,
            entrypoint=entrypoint,
            env=env,
            resource_limits=resource_limits,
        )
        
        # Build shared volume for execd
        volumes = [
            {
                "name": "opensandbox-bin",
                "emptyDir": {}
            }
        ]
        
        # Build runtime-generated BatchSandbox manifest
        # This contains only the essential runtime fields
        runtime_manifest = {
            "apiVersion": f"{self.group}/{self.version}",
            "kind": "BatchSandbox",
            "metadata": {
                "name": batchsandbox_name,
                "namespace": namespace,
                "labels": labels,
            },
            "spec": {
                "replicas": 1,
                "expireTime": expires_at.isoformat(),
                "template": {
                    "spec": {
                        "initContainers": [self._container_to_dict(init_container)],
                        "containers": [self._container_to_dict(main_container)],
                        "volumes": volumes,
                    }
                },
            },
        }
        
        # Merge with template to get final manifest
        batchsandbox = self.template_manager.merge_with_runtime_values(runtime_manifest)
        self._merge_pod_spec_extras(batchsandbox, extra_volumes, extra_mounts)
        
        # Create BatchSandbox
        created = self.custom_api.create_namespaced_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            body=batchsandbox,
        )
        
        return {
            "name": created["metadata"]["name"],
            "uid": created["metadata"]["uid"],
        }
    
    def _create_workload_from_pool(
        self,
        batchsandbox_name: str,
        namespace: str,
        labels: Dict[str, str],
        pool_ref: str,
        expires_at: datetime,
        entrypoint: List[str],
        env: Dict[str, str],
    ) -> Dict[str, Any]:
        """
        Create BatchSandbox workload from a pre-warmed resource pool.
        
        Pool-based creation uses poolRef to reference an existing pool.
        The pool already defines the pod template, so no additional template is needed.
        Only entrypoint and env can be customized.
        
        Args:
            batchsandbox_name: Name of the BatchSandbox resource
            namespace: Kubernetes namespace
            labels: Labels to apply
            pool_ref: Reference to the resource pool
            expires_at: Expiration time
            entrypoint: Container entrypoint command (can be customized)
            env: Environment variables (can be customized)
            
        Returns:
            Dict with 'name' and 'uid' of created BatchSandbox
            
        Raises:
            SandboxError: If required parameters are invalid
        """
        runtime_manifest = {
            "apiVersion": f"{self.group}/{self.version}",
            "kind": "BatchSandbox",
            "metadata": {
                "name": batchsandbox_name,
                "namespace": namespace,
                "labels": labels,
            },
            "spec": {
                "replicas": 1,
                "poolRef": pool_ref,
                "expireTime": expires_at.isoformat(),
                "taskTemplate": self._build_task_template(entrypoint, env),
            },
        }
        
        # Pool-based creation does not need template merging
        # Create BatchSandbox directly
        created = self.custom_api.create_namespaced_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            body=runtime_manifest,
        )
        
        return {
            "name": created["metadata"]["name"],
            "uid": created["metadata"]["uid"],
        }

    def _extract_template_pod_extras(self) -> tuple[list[Dict[str, Any]], list[Dict[str, Any]]]:
        """
        Extract extra volumes and volume mounts from the BatchSandbox template.

        Only these fields are supported here because runtime manifests must
        always inject execd init container, main container, and volumes.
        """
        template = self.template_manager.get_base_template()
        spec = template.get("spec", {}) if isinstance(template, dict) else {}
        template_spec = spec.get("template", {}).get("spec", {})
        extra_volumes = template_spec.get("volumes", []) or []

        extra_mounts: list[Dict[str, Any]] = []
        containers = template_spec.get("containers", []) or []
        if containers:
            # Prefer container named "sandbox" if present, otherwise first container.
            target = None
            for container in containers:
                if container.get("name") == "sandbox":
                    target = container
                    break
            if target is None:
                target = containers[0]
            extra_mounts = target.get("volumeMounts", []) or []

        if not isinstance(extra_volumes, list):
            extra_volumes = []
        if not isinstance(extra_mounts, list):
            extra_mounts = []
        return extra_volumes, extra_mounts

    def _merge_pod_spec_extras(
        self,
        batchsandbox: Dict[str, Any],
        extra_volumes: list[Dict[str, Any]],
        extra_mounts: list[Dict[str, Any]],
    ) -> None:
        """
        Merge extra volumes/volumeMounts into the runtime-generated pod spec.

        This keeps execd injections intact while allowing user templates to
        provide additional read-only mounts (e.g., shared skills directory).
        """
        try:
            spec = batchsandbox["spec"]["template"]["spec"]
        except KeyError:
            return

        # Merge volumes by name (do not overwrite existing runtime volumes).
        volumes = spec.get("volumes", []) or []
        if isinstance(volumes, list) and extra_volumes:
            existing = {v.get("name") for v in volumes if isinstance(v, dict)}
            for vol in extra_volumes:
                if not isinstance(vol, dict):
                    continue
                name = vol.get("name")
                if not name or name in existing:
                    continue
                volumes.append(vol)
                existing.add(name)
            spec["volumes"] = volumes

        # Merge volumeMounts into the main container (index 0).
        containers = spec.get("containers", []) or []
        if not containers or not isinstance(containers, list):
            return
        main_container = containers[0]
        mounts = main_container.get("volumeMounts", []) or []
        if isinstance(mounts, list) and extra_mounts:
            existing = {m.get("name") for m in mounts if isinstance(m, dict)}
            for mnt in extra_mounts:
                if not isinstance(mnt, dict):
                    continue
                name = mnt.get("name")
                if not name or name in existing:
                    continue
                mounts.append(mnt)
                existing.add(name)
            main_container["volumeMounts"] = mounts

    # Todo support empty cmd or env
    def _build_task_template(
        self,
        entrypoint: List[str],
        env: Dict[str, str],
    ) -> Dict[str, Any]:
        """
        Build taskTemplate for pool-based BatchSandbox.
        
        In pool mode, task should use bootstrap.sh to start execd and business process.
        
        Generated command example:
            /bin/sh -c "/opt/opensandbox/bin/bootstrap.sh python app.py &"
        
        Note: All entrypoint arguments are properly shell-escaped using shlex.quote
        to prevent shell injection and preserve arguments with spaces or special characters.
        
        Args:
            entrypoint: Container entrypoint command
            env: Environment variables
            
        Returns:
            Dict: taskTemplate specification with TaskSpec structure
        """
        # Build command: execute bootstrap.sh with entrypoint in background
        # Use shlex.quote to safely escape each entrypoint argument to prevent shell injection
        escaped_entrypoint = ' '.join(shlex.quote(arg) for arg in entrypoint)
        user_process_cmd = f"/opt/opensandbox/bin/bootstrap.sh {escaped_entrypoint} &"
        
        wrapped_command = ["/bin/sh", "-c", user_process_cmd]
        
        # Convert env dict to k8s EnvVar format
        env_list = [{"name": k, "value": v} for k, v in env.items()] if env else []
        
        # Return TaskTemplateSpec structure
        return {
            "spec": {
                "process": {
                    "command": wrapped_command,
                    "env": env_list,
                }
            }
        }
    
    def _build_execd_init_container(self, execd_image: str) -> V1Container:
        """
        Build init container for execd installation.
        
        This init container copies execd binary and bootstrap.sh script from
        execd image to shared volume, making them available to the main container.
        
        The bootstrap.sh script (from execd image) will:
        - Start execd in background (redirects logs to /tmp/execd.log)
        - Use exec to replace current process with user's command
        
        Args:
            execd_image: execd container image
            
        Returns:
            V1Container: Init container spec
        """
        # Copy execd binary and bootstrap.sh from image to shared volume
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
                    mount_path="/opt/opensandbox/bin"
                )
            ],
        )
    
    def _build_main_container(
        self,
        image_spec: ImageSpec,
        entrypoint: List[str],
        env: Dict[str, str],
        resource_limits: Dict[str, str],
    ) -> V1Container:
        """
        Build main container spec with execd support.
        
        The container will use bootstrap script to start execd in background,
        then execute user's command.
        
        Args:
            image_spec: Container image specification
            entrypoint: Container entrypoint command
            env: Environment variables
            resource_limits: Resource limits
            
        Returns:
            V1Container: Main container spec
        """
        # Convert env dict to V1EnvVar list and inject EXECD path
        env_vars = [V1EnvVar(name=k, value=v) for k, v in env.items()]
        # Add EXECD environment variable to specify execd binary path
        env_vars.append(V1EnvVar(name="EXECD", value="/opt/opensandbox/bin/execd"))
        
        # Build resource requirements
        resources = None
        if resource_limits:
            resources = V1ResourceRequirements(
                limits=resource_limits,
                requests=resource_limits,  # Set requests = limits for guaranteed QoS
            )
        
        # Wrap entrypoint with bootstrap script to start execd
        wrapped_command = ["/opt/opensandbox/bin/bootstrap.sh"] + entrypoint
        
        return V1Container(
            name="sandbox",
            image=image_spec.uri,
            command=wrapped_command,
            env=env_vars if env_vars else None,
            resources=resources,
            volume_mounts=[
                V1VolumeMount(
                    name="opensandbox-bin",
                    mount_path="/opt/opensandbox/bin"
                )
            ],
        )
    
    def _container_to_dict(self, container: V1Container) -> Dict[str, Any]:
        """
        Convert V1Container to dict for CRD.
        
        Args:
            container: V1Container object
            
        Returns:
            Dict representation of container
        """
        result = {
            "name": container.name,
            "image": container.image,
        }
        
        if container.command:
            result["command"] = container.command
        
        if container.args:
            result["args"] = container.args
        
        if container.env:
            result["env"] = [
                {"name": e.name, "value": e.value}
                for e in container.env
            ]
        
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
        """Get BatchSandbox by sandbox ID."""
        label_selector = f"{SANDBOX_ID_LABEL}={sandbox_id}"
        
        try:
            batchsandbox_list = self.custom_api.list_namespaced_custom_object(
                group=self.group,
                version=self.version,
                namespace=namespace,
                plural=self.plural,
                label_selector=label_selector,
            )
            
            if batchsandbox_list.get("items"):
                return batchsandbox_list["items"][0]
            return None
        except ApiException as e:
            # Handle 404 when CRD doesn't exist or no resources found
            if e.status == 404:
                return None
            # Re-raise other API exceptions
            raise
        except Exception as e:
            # Log unexpected errors and re-raise
            logger.error(f"Unexpected error getting BatchSandbox for {sandbox_id}: {e}")
            raise
    
    def delete_workload(self, sandbox_id: str, namespace: str) -> None:
        """Delete BatchSandbox workload."""
        batchsandbox = self.get_workload(sandbox_id, namespace)
        if not batchsandbox:
            raise Exception(f"BatchSandbox for sandbox {sandbox_id} not found")
        
        self.custom_api.delete_namespaced_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            name=batchsandbox["metadata"]["name"],
            grace_period_seconds=0,
        )
    
    def list_workloads(self, namespace: str, label_selector: str) -> List[Dict[str, Any]]:
        """List BatchSandboxes matching label selector."""
        try:
            batchsandbox_list = self.custom_api.list_namespaced_custom_object(
                group=self.group,
                version=self.version,
                namespace=namespace,
                plural=self.plural,
                label_selector=label_selector,
            )
            return batchsandbox_list.get("items", [])
        except ApiException as e:
            # Handle 404 when CRD doesn't exist
            if e.status == 404:
                return []
            # Re-raise other API exceptions
            raise
        except Exception as e:
            # Log and re-raise unexpected errors
            logger.error(f"Unexpected error listing BatchSandboxes: {e}")
            raise
    
    def update_expiration(self, sandbox_id: str, namespace: str, expires_at: datetime) -> None:
        """Update BatchSandbox expiration time.
        
        Args:
            sandbox_id: Sandbox ID
            namespace: Kubernetes namespace
            expires_at: New expiration time
            
        Raises:
            Exception: If BatchSandbox not found or update fails
        """
        batchsandbox = self.get_workload(sandbox_id, namespace)
        if not batchsandbox:
            raise Exception(f"BatchSandbox for sandbox {sandbox_id} not found")
        
        # Patch BatchSandbox spec.expireTime
        body = {
            "spec": {
                "expireTime": expires_at.isoformat()
            }
        }
        
        self.custom_api.patch_namespaced_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            name=batchsandbox["metadata"]["name"],
            body=body,
        )
    
    def get_expiration(self, workload: Dict[str, Any]) -> Optional[datetime]:
        """Get expiration time from BatchSandbox.
        
        Args:
            workload: BatchSandbox dict
            
        Returns:
            Expiration datetime or None if not set or invalid
        """
        spec = workload.get("spec", {})
        expire_time_str = spec.get("expireTime")
        
        if not expire_time_str:
            return None
        
        try:
            # Parse ISO format datetime
            return datetime.fromisoformat(expire_time_str.replace('Z', '+00:00'))
        except (ValueError, TypeError) as e:
            logger.warning(f"Invalid expireTime format: {expire_time_str}, error: {e}")
            return None
    
    def get_status(self, workload: Dict[str, Any]) -> Dict[str, Any]:
        """
        Get status from BatchSandbox.
        
        The status is derived from the BatchSandbox status fields:
        - replicas: total number of pods
        - allocated: number of scheduled pods
        - ready: number of ready pods
        """
        status = workload.get("status", {})
        
        replicas = status.get("replicas", 0)
        ready = status.get("ready", 0)
        allocated = status.get("allocated", 0)
        
        # Get annotations for endpoint information
        annotations = workload.get("metadata", {}).get("annotations", {})
        endpoints_str = annotations.get("sandbox.opensandbox.io/endpoints")
        
        # Determine state based on ready status and endpoint availability
        if ready == 1 and endpoints_str:
            # Pod is ready and has an IP address assigned
            state = "Running"
            reason = "READY_WITH_IP"
            message = f"Pod is ready with IP assigned ({ready}/{replicas} ready)"
        elif ready > 0:
            # Pod is ready but no IP yet - still pending
            state = "Pending"
            reason = "POD_READY_NO_IP"
            message = f"Pod is ready but waiting for IP assignment ({ready}/{replicas} ready)"
        elif allocated > 0:
            # Pod is allocated/scheduled but not ready yet
            state = "Pending"
            reason = "POD_SCHEDULED"
            message = f"Pod is scheduled but not ready ({allocated}/{replicas} allocated, {ready} ready)"
        else:
            # Pod is not allocated yet
            state = "Pending"
            reason = "BATCHSANDBOX_PENDING"
            message = "BatchSandbox is pending allocation"
        
        # Get creation timestamp
        creation_timestamp = workload.get("metadata", {}).get("creationTimestamp")
        
        return {
            "state": state,
            "reason": reason,
            "message": message,
            "last_transition_at": creation_timestamp,
        }
    
    def get_endpoint_info(self, workload: Dict[str, Any], port: int) -> Optional[str]:
        """
        Get endpoint information from BatchSandbox.
        
        Reads Pod IP from sandbox.opensandbox.io/endpoints annotation.
        The annotation contains a JSON array of IP addresses.
        
        Args:
            workload: BatchSandbox dict
            port: Port number
            
        Returns:
            Endpoint string in format "IP:PORT" or None if not available
        """
        import json
        
        # Get annotations
        annotations = workload.get("metadata", {}).get("annotations", {})
        
        # Get endpoints from annotation
        endpoints_str = annotations.get("sandbox.opensandbox.io/endpoints")
        if not endpoints_str:
            return None
        
        try:
            # Parse JSON array of IPs
            endpoints = json.loads(endpoints_str)
            if endpoints and len(endpoints) > 0:
                # Use the first IP
                pod_ip = endpoints[0]
                return f"{pod_ip}:{port}"
        except (json.JSONDecodeError, IndexError, TypeError):
            return None
        
        return None

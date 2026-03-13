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
import json
import shlex
from datetime import datetime
from typing import Dict, List, Any, Optional

from kubernetes.client import (
    V1Container,
    V1EnvVar,
    V1ResourceRequirements,
    V1VolumeMount,
)

from src.config import AppConfig, INGRESS_MODE_GATEWAY
from src.services.helpers import format_ingress_endpoint
from src.api.schema import Endpoint, ImageSpec, NetworkPolicy, Volume
from src.services.k8s.image_pull_secret_helper import (
    build_image_pull_secret,
    build_image_pull_secret_name,
)
from src.services.k8s.batchsandbox_template import BatchSandboxTemplateManager
from src.services.k8s.client import K8sClient
from src.services.k8s.egress_helper import (
    apply_egress_to_spec,
    build_security_context_for_sandbox_container,
    build_security_context_from_dict,
    serialize_security_context_to_dict,
)
from src.services.k8s.volume_helper import apply_volumes_to_pod_spec
from src.services.k8s.workload_provider import WorkloadProvider
from src.services.runtime_resolver import SecureRuntimeResolver

logger = logging.getLogger(__name__)


class BatchSandboxProvider(WorkloadProvider):
    """
    Workload provider using BatchSandbox CRD.
    
    BatchSandbox is a custom resource that manages Pod lifecycle
    and provides additional features like task management.
    """
    
    def __init__(
        self,
        k8s_client: K8sClient,
        app_config: Optional[AppConfig] = None,
    ):
        """
        Initialize BatchSandbox provider.

        Args:
            k8s_client: Kubernetes client wrapper
            app_config: Application config; kubernetes/ingress sub-configs are read from it directly.
        """
        self.k8s_client = k8s_client
        self.ingress_config = app_config.ingress if app_config else None

        k8s_config = app_config.kubernetes if app_config else None
        template_file_path = k8s_config.batchsandbox_template_file if k8s_config else None
        if template_file_path:
            logger.info("Using BatchSandbox template file: %s", template_file_path)
        self.execd_init_resources = k8s_config.execd_init_resources if k8s_config else None

        # Initialize secure runtime resolver
        self.resolver = SecureRuntimeResolver(app_config) if app_config else None
        self.runtime_class = (
            self.resolver.get_k8s_runtime_class() if self.resolver else None
        )

        # CRD constants
        self.group = "sandbox.opensandbox.io"
        self.version = "v1alpha1"
        self.plural = "batchsandboxes"
        
        # Template manager
        self.template_manager = BatchSandboxTemplateManager(template_file_path)

    def supports_image_auth(self) -> bool:
        """BatchSandbox supports image pull auth via imagePullSecrets injection."""
        return True

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
            network_policy: Optional network policy for egress traffic control.
                When provided, an egress sidecar container will be added to the Pod.
            egress_image: Container image for the egress sidecar (required when network_policy is set).
            volumes: Optional list of volume mounts for the sandbox.

        Returns:
            Dict with 'name' and 'uid' of created BatchSandbox

        Raises:
            SandboxError: If pool mode is used with volumes (not supported).
        """
        extensions = extensions or {}

        # Log runtime class usage for debugging
        if self.runtime_class:
            logger.info(
                "Using Kubernetes RuntimeClass '%s' for sandbox %s",
                self.runtime_class,
                sandbox_id,
            )

        # If poolRef is provided and not empty, create workload from pool
        if extensions.get("poolRef"):
            # Pool mode does not support volumes
            if volumes:
                raise ValueError(
                    "Pool mode does not support volumes. "
                    "Remove 'volumes' from request or use template mode."
                )
            # When using pool, only entrypoint and env can be customized
            return self._create_workload_from_pool(
                batchsandbox_name=sandbox_id,
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
            has_network_policy=network_policy is not None,
        )
        
        # Build containers list
        containers = [self._container_to_dict(main_container)]
        
        # Build base pod spec
        pod_spec: Dict[str, Any] = {
            "initContainers": [self._container_to_dict(init_container)],
            "containers": containers,
            "volumes": [
                {
                    "name": "opensandbox-bin",
                    "emptyDir": {}
                }
            ],
        }

        # Inject runtimeClassName if secure runtime is configured
        if self.runtime_class:
            pod_spec["runtimeClassName"] = self.runtime_class

        # Inject imagePullSecrets if image auth is provided
        # secret_name is deterministic so it can be embedded before the Secret is created
        if image_spec.auth:
            secret_name = build_image_pull_secret_name(sandbox_id)
            pod_spec["imagePullSecrets"] = [{"name": secret_name}]

        # Add egress sidecar if network policy is provided
        apply_egress_to_spec(
            pod_spec=pod_spec,
            containers=containers,
            network_policy=network_policy,
            egress_image=egress_image,
        )

        # Add user-specified volumes if provided
        if volumes:
            apply_volumes_to_pod_spec(pod_spec, volumes)

        # Build runtime-generated BatchSandbox manifest
        # This contains only the essential runtime fields
        runtime_manifest = {
            "apiVersion": f"{self.group}/{self.version}",
            "kind": "BatchSandbox",
            "metadata": {
                "name": sandbox_id,
                "namespace": namespace,
                "labels": labels,
            },
            "spec": {
                "replicas": 1,
                "expireTime": expires_at.isoformat(),
                "template": {
                    "spec": pod_spec,
                },
            },
        }
        
        # Merge with template to get final manifest
        batchsandbox = self.template_manager.merge_with_runtime_values(runtime_manifest)
        self._merge_pod_spec_extras(batchsandbox, extra_volumes, extra_mounts)
        
        # Create BatchSandbox
        created = self.k8s_client.create_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            body=batchsandbox,
        )

        # Create imagePullSecret with ownerReference pointing to the BatchSandbox
        if image_spec.auth:
            secret = build_image_pull_secret(
                sandbox_id=sandbox_id,
                image_uri=image_spec.uri,
                auth=image_spec.auth,
                owner_uid=created["metadata"]["uid"],
                owner_api_version=f"{self.group}/{self.version}",
                owner_kind="BatchSandbox",
            )
            try:
                self.k8s_client.create_secret(namespace=namespace, body=secret)
                logger.info("Created imagePullSecret for sandbox %s", sandbox_id)
            except Exception:
                logger.warning("Failed to create imagePullSecret for sandbox %s, rolling back BatchSandbox", sandbox_id)
                try:
                    self.k8s_client.delete_custom_object(
                        group=self.group,
                        version=self.version,
                        namespace=namespace,
                        plural=self.plural,
                        name=sandbox_id,
                        grace_period_seconds=0,
                    )
                except Exception as del_exc:
                    logger.warning("Failed to rollback BatchSandbox %s: %s", sandbox_id, del_exc)
                raise

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
        created = self.k8s_client.create_custom_object(
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

    # TODO: support empty cmd or env
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
                    mount_path="/opt/opensandbox/bin"
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
        has_network_policy: bool = False,
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
            has_network_policy: Whether network policy is enabled for this sandbox
            
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
            volume_mounts=[
                V1VolumeMount(
                    name="opensandbox-bin",
                    mount_path="/opt/opensandbox/bin"
                )
            ],
            security_context=security_context,
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
        
        if container.security_context:
            security_context_dict = serialize_security_context_to_dict(container.security_context)
            if security_context_dict:
                result["securityContext"] = security_context_dict
        
        return result

    def get_workload(self, sandbox_id: str, namespace: str) -> Optional[Dict[str, Any]]:
        """Get BatchSandbox by sandbox ID."""
        workload = self.k8s_client.get_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            name=sandbox_id,
        )
        if workload:
            return workload

        # Fallback for pre-upgrade sandboxes that used "sandbox-<id>" naming
        legacy_name = self.legacy_resource_name(sandbox_id)
        if legacy_name != sandbox_id:
            return self.k8s_client.get_custom_object(
                group=self.group,
                version=self.version,
                namespace=namespace,
                plural=self.plural,
                name=legacy_name,
            )

        return None
    
    def delete_workload(self, sandbox_id: str, namespace: str) -> None:
        """Delete BatchSandbox workload."""
        batchsandbox = self.get_workload(sandbox_id, namespace)
        if not batchsandbox:
            raise Exception(f"BatchSandbox for sandbox {sandbox_id} not found")
        
        self.k8s_client.delete_custom_object(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            name=batchsandbox["metadata"]["name"],
            grace_period_seconds=0,
        )
    
    def list_workloads(self, namespace: str, label_selector: str) -> List[Dict[str, Any]]:
        """List BatchSandboxes matching label selector."""
        return self.k8s_client.list_custom_objects(
            group=self.group,
            version=self.version,
            namespace=namespace,
            plural=self.plural,
            label_selector=label_selector,
        )
    
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
        
        self.k8s_client.patch_custom_object(
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
            logger.warning("Invalid expireTime format: %s, error: %s", expire_time_str, e)
            return None
    
    def _parse_pod_ip(self, workload: Dict[str, Any]) -> Optional[str]:
        """Parse the first Pod IP from the endpoints annotation.

        Returns the IP string if the annotation exists and contains a non-empty
        JSON array, otherwise returns None.
        """
        annotations = workload.get("metadata", {}).get("annotations", {})
        endpoints_str = annotations.get("sandbox.opensandbox.io/endpoints")
        if not endpoints_str:
            return None
        try:
            endpoints = json.loads(endpoints_str)
            if endpoints and len(endpoints) > 0:
                return endpoints[0]
        except (json.JSONDecodeError, IndexError, TypeError):
            pass
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

        pod_ip = self._parse_pod_ip(workload)

        # Determine state: Pending -> Allocated (IP assigned) -> Running (Pod ready)
        if ready == 1 and pod_ip:
            # Pod is ready and has IP
            state = "Running"
            reason = "POD_READY_WITH_IP"
            message = f"Pod is ready with IP ({ready}/{replicas} ready)"
        elif pod_ip:
            # Pod has IP assigned but not ready yet
            state = "Allocated"
            reason = "IP_ASSIGNED"
            message = f"Pod has IP assigned but not ready ({allocated}/{replicas} allocated, {ready} ready)"
        else:
            # Pod is not allocated yet or allocated but no IP
            state = "Pending"
            reason = "POD_SCHEDULED" if allocated > 0 else "BATCHSANDBOX_PENDING"
            message = (
                f"Pod is scheduled but waiting for IP ({allocated}/{replicas} allocated, {ready} ready)"
                if allocated > 0
                else "BatchSandbox is pending allocation"
            )
        
        # Get creation timestamp
        creation_timestamp = workload.get("metadata", {}).get("creationTimestamp")
        
        return {
            "state": state,
            "reason": reason,
            "message": message,
            "last_transition_at": creation_timestamp,
        }
    
    def get_endpoint_info(self, workload: Dict[str, Any], port: int, sandbox_id: str) -> Optional[Endpoint]:
        """
        Get endpoint information from BatchSandbox.
        - gateway mode: use ingress config to format endpoint
        - direct/default: resolve Pod IP from annotation
        """
        if self.ingress_config and self.ingress_config.mode == INGRESS_MODE_GATEWAY:
            return format_ingress_endpoint(self.ingress_config, sandbox_id, port)

        pod_ip = self._parse_pod_ip(workload)
        if not pod_ip:
            return None
        return Endpoint(endpoint=f"{pod_ip}:{port}")

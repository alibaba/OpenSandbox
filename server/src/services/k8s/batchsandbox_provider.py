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
    
    def _build_task_template(
        self,
        entrypoint: List[str],
        env: Dict[str, str],
    ) -> Dict[str, Any]:
        """
        Build taskTemplate for pool-based BatchSandbox.
        
        In pool mode, task should:
        1. Inject environment variables to .env file
        2. Use bootstrap.sh to start execd and business process (same as non-pool)
        
        Generated command example with env:
            /bin/sh -c "printf '%s=%s\\n' 'KEY1' 'value1' >> ${EXECD_ENVS:-/opt/opensandbox/.env} && 
                        printf '%s=%s\\n' 'KEY2' 'value2' >> ${EXECD_ENVS:-/opt/opensandbox/.env} && 
                        /opt/opensandbox/execd/bootstrap.sh /usr/bin/python app.py"
        
        Generated command example without env:
            /bin/sh -c "/opt/opensandbox/execd/bootstrap.sh /usr/bin/python app.py"
        
        Args:
            entrypoint: Container entrypoint command
            env: Environment variables
            
        Returns:
            Dict: taskTemplate specification with TaskSpec structure
        """
        # Build shell command to write env to .env file and then execute bootstrap.sh
        # Get env file path from EXECD_ENVS, default to /opt/opensandbox/.env
        env_file_expr = "${EXECD_ENVS:-/opt/opensandbox/.env}"
        
        # Generate env file write commands (append mode)
        env_commands = []
        if env:
            for key, value in env.items():
                if value is None:
                    continue  # Skip None values
                env_commands.append(f"printf '%s=%s\\n' '{key}' '{value}' >> {env_file_expr}")
        
        # Build final command: append env vars to .env file, then execute bootstrap.sh with entrypoint
        # Commands are joined with && to ensure sequential execution
        commands = env_commands + [
            f"/opt/opensandbox/execd/bootstrap.sh {' '.join(entrypoint)}"
        ]
        
        wrapped_command = ["/bin/sh", "-c", " && ".join(commands)]
        
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
        
        This init container:
        1. Copies execd binary to shared volume
        2. Generates bootstrap.sh script with following content:
           - Start execd in background (redirects logs to /tmp/execd.log)
           - Use exec to replace current process with user's command
        
        Generated bootstrap.sh:
            #!/bin/sh
            set -e
            /opt/opensandbox/execd/execd >/tmp/execd.log 2>&1 &
            exec "$@"
        
        Args:
            execd_image: execd container image
            
        Returns:
            V1Container: Init container spec
        """
        # Build the script with proper shell syntax
        script = (
            "cp ./execd /opt/opensandbox/execd/execd && "
            "chmod +x /opt/opensandbox/execd/execd && "
            "cat > /opt/opensandbox/execd/bootstrap.sh << 'BOOTSTRAP_EOF'\n"
            "#!/bin/sh\n"
            "set -e\n"
            "/opt/opensandbox/execd/execd >/tmp/execd.log 2>&1 &\n"
            'exec "$@"\n'
            "BOOTSTRAP_EOF\n"
            "chmod +x /opt/opensandbox/execd/bootstrap.sh"
        )
        
        return V1Container(
            name="execd-installer",
            image=execd_image,
            command=["/bin/sh", "-c"],
            args=[script],
            volume_mounts=[
                V1VolumeMount(
                    name="opensandbox-bin",
                    mount_path="/opt/opensandbox/execd"
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
        # Convert env dict to V1EnvVar list
        env_vars = [V1EnvVar(name=k, value=v) for k, v in env.items()]
        
        # Build resource requirements
        resources = None
        if resource_limits:
            resources = V1ResourceRequirements(
                limits=resource_limits,
                requests=resource_limits,  # Set requests = limits for guaranteed QoS
            )
        
        # Wrap entrypoint with bootstrap script to start execd
        wrapped_command = ["/opt/opensandbox/execd/bootstrap.sh"] + entrypoint
        
        return V1Container(
            name="sandbox",
            image=image_spec.uri,
            command=wrapped_command,
            env=env_vars if env_vars else None,
            resources=resources,
            volume_mounts=[
                V1VolumeMount(
                    name="opensandbox-bin",
                    mount_path="/opt/opensandbox/execd"
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

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
SandboxSnapshot CRD provider for pause/resume operations.
"""

import logging
from typing import Any, Dict, List, Optional

from kubernetes.client import ApiException
from opensandbox_server.services.k8s.client import K8sClient

logger = logging.getLogger(__name__)


class SandboxSnapshotProvider:
    """Provider for SandboxSnapshot CRD operations."""

    GROUP = "sandbox.opensandbox.io"
    VERSION = "v1alpha1"
    PLURAL = "sandboxsnapshots"

    def __init__(self, k8s_client: K8sClient):
        """Initialize provider with K8sClient.

        Args:
            k8s_client: Kubernetes client for API operations.
        """
        self.k8s_client = k8s_client

    def get_snapshot(
        self,
        snapshot_name: str,
        namespace: str,
    ) -> Optional[Dict[str, Any]]:
        """Get SandboxSnapshot by name.

        Args:
            snapshot_name: Name of the SandboxSnapshot.
            namespace: Kubernetes namespace.

        Returns:
            SandboxSnapshot dict if found, None if not found.
        """
        try:
            return self.k8s_client.get_custom_object(
                group=self.GROUP,
                version=self.VERSION,
                namespace=namespace,
                plural=self.PLURAL,
                name=snapshot_name,
            )
        except ApiException as e:
            if e.status == 404:
                return None
            logger.warning(
                "Failed to get SandboxSnapshot %s/%s: %s",
                namespace,
                snapshot_name,
                e,
            )
            raise

    def create_snapshot(
        self,
        namespace: str,
        body: Dict[str, Any],
    ) -> Dict[str, Any]:
        """Create a SandboxSnapshot.

        Args:
            namespace: Kubernetes namespace.
            body: SandboxSnapshot manifest.

        Returns:
            Created SandboxSnapshot.
        """
        return self.k8s_client.create_custom_object(
            group=self.GROUP,
            version=self.VERSION,
            namespace=namespace,
            plural=self.PLURAL,
            body=body,
        )

    def patch_snapshot_spec(
        self,
        snapshot_name: str,
        namespace: str,
        spec_patch: Dict[str, Any],
    ) -> Dict[str, Any]:
        """Patch SandboxSnapshot spec.

        Args:
            snapshot_name: Name of the SandboxSnapshot.
            namespace: Kubernetes namespace.
            spec_patch: Spec fields to patch (merged with existing spec).

        Returns:
            Updated SandboxSnapshot.
        """
        body = {"spec": spec_patch}
        return self.k8s_client.patch_custom_object(
            group=self.GROUP,
            version=self.VERSION,
            namespace=namespace,
            plural=self.PLURAL,
            name=snapshot_name,
            body=body,
        )

    def delete_snapshot(
        self,
        snapshot_name: str,
        namespace: str,
    ) -> None:
        """Delete a SandboxSnapshot.

        Args:
            snapshot_name: Name of the SandboxSnapshot.
            namespace: Kubernetes namespace.
        """
        try:
            self.k8s_client.delete_custom_object(
                group=self.GROUP,
                version=self.VERSION,
                namespace=namespace,
                plural=self.PLURAL,
                name=snapshot_name,
            )
        except ApiException as e:
            if e.status == 404:
                return  # Already deleted
            logger.warning(
                "Failed to delete SandboxSnapshot %s/%s: %s",
                namespace,
                snapshot_name,
                e,
            )
            raise

    def list_snapshots(
        self,
        namespace: str,
        label_selector: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """List SandboxSnapshots in namespace.

        Args:
            namespace: Kubernetes namespace.
            label_selector: Optional label selector to filter results.

        Returns:
            List of SandboxSnapshot dicts.
        """
        return self.k8s_client.list_custom_objects(
            group=self.GROUP,
            version=self.VERSION,
            namespace=namespace,
            plural=self.PLURAL,
            label_selector=label_selector,
        )
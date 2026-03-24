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
Volume helper utilities for Kubernetes pod specs.
"""

import logging
from typing import Any, Dict, List

from opensandbox_server.api.schema import Volume

logger = logging.getLogger(__name__)


def apply_volumes_to_pod_spec(
    pod_spec: Dict[str, Any],
    volumes: List[Volume],
) -> None:
    """
    Apply user-specified volumes to a Kubernetes pod spec.

    This function converts Volume API objects to Kubernetes volume and volumeMount
    definitions and adds them to the pod spec in-place.

    Currently supported backends:
    - pvc: Maps to Kubernetes PersistentVolumeClaim
    - host: Maps to Kubernetes hostPath volume

    Args:
        pod_spec: The pod spec dictionary to modify in-place
        volumes: List of Volume API objects

    Raises:
        ValueError: If an unsupported volume backend is specified
    """
    containers = pod_spec.get("containers", [])
    if not containers:
        logger.warning("No containers in pod spec, skipping volume mounts")
        return

    main_container = containers[0]
    mounts = main_container.get("volumeMounts", [])
    pod_volumes = pod_spec.get("volumes", [])

    # Collect existing volume names to prevent collisions with internal volumes
    existing_volume_names = {v.get("name") for v in pod_volumes if isinstance(v, dict)}
    # One Kubernetes volume per unique PVC; multiple volumeMounts can reference it
    pvc_to_volume_name: Dict[str, str] = {}

    for vol in volumes:
        vol_name = vol.name

        # Check for collision with internal volumes
        if vol_name in existing_volume_names:
            raise ValueError(
                f"Volume name '{vol_name}' conflicts with an internal volume. "
                "Please use a different volume name."
            )

        if vol.pvc is not None:
            # PVC backend: maps to Kubernetes PersistentVolumeClaim.
            # Multiple Volume API objects sharing the same claim_name must produce
            # a single Kubernetes volume and multiple volumeMounts (CSI drivers
            # can fail when the same PVC is defined in multiple volume entries).
            pvc_claim_name = vol.pvc.claim_name

            if pvc_claim_name not in pvc_to_volume_name:
                # First use of this PVC: create one volume, use current vol.name as volume name
                pod_volumes.append({
                    "name": vol_name,
                    "persistentVolumeClaim": {
                        "claimName": pvc_claim_name,
                    },
                })
                pvc_to_volume_name[pvc_claim_name] = vol_name
                existing_volume_names.add(vol_name)

            mount = {
                "name": pvc_to_volume_name[pvc_claim_name],
                "mountPath": vol.mount_path,
                "readOnly": vol.read_only,
            }
            if vol.sub_path:
                mount["subPath"] = vol.sub_path
            mounts.append(mount)

            logger.info(
                f"Added PVC volume '{vol_name}' (claim: {pvc_claim_name}) mounted at '{vol.mount_path}' for sandbox"
            )
        elif vol.host is not None:
            # Host backend: maps to hostPath volume
            # Note: hostPath is node-local and not recommended for production
            host_path = vol.host.path

            pod_volumes.append({
                "name": vol_name,
                "hostPath": {
                    "path": host_path,
                    "type": "DirectoryOrCreate",
                },
            })

            mount = {
                "name": vol_name,
                "mountPath": vol.mount_path,
                "readOnly": vol.read_only,
            }
            if vol.sub_path:
                mount["subPath"] = vol.sub_path
            mounts.append(mount)

            logger.info(
                f"Added hostPath volume '{vol_name}' (path: {host_path}) mounted at '{vol.mount_path}' for sandbox"
            )
        else:
            raise ValueError(
                f"Volume '{vol_name}' has no supported backend specified. "
                "Supported backends: pvc, host"
            )

    # Update pod spec with modified volumes and mounts
    pod_spec["volumes"] = pod_volumes
    main_container["volumeMounts"] = mounts

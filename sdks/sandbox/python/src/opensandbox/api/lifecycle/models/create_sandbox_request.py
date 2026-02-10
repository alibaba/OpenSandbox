#
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
#

from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar, cast

from attrs import define as _attrs_define
from attrs import field as _attrs_field

from ..types import UNSET, Unset

if TYPE_CHECKING:
    from ..models.create_sandbox_request_env import CreateSandboxRequestEnv
    from ..models.create_sandbox_request_extensions import CreateSandboxRequestExtensions
    from ..models.create_sandbox_request_metadata import CreateSandboxRequestMetadata
    from ..models.image_spec import ImageSpec
    from ..models.network_policy import NetworkPolicy
    from ..models.resource_limits import ResourceLimits
    from ..models.volume import Volume


T = TypeVar("T", bound="CreateSandboxRequest")


@_attrs_define
class CreateSandboxRequest:
    """Request to create a new sandbox from a container image.

    **Note**: API Key authentication is required via the `OPEN-SANDBOX-API-KEY` header.

        Attributes:
            image (ImageSpec): Container image specification for sandbox provisioning.

                Supports public registry images and private registry images with authentication.
            timeout (int): Sandbox timeout in seconds. The sandbox will automatically terminate after this duration.
                SDK clients should provide a default value (e.g., 3600 seconds / 1 hour).
            resource_limits (ResourceLimits): Runtime resource constraints as key-value pairs. Similar to Kubernetes
                resource specifications,
                allows flexible definition of resource limits. Common resource types include:
                - `cpu`: CPU allocation in millicores (e.g., "250m" for 0.25 CPU cores)
                - `memory`: Memory allocation in bytes or human-readable format (e.g., "512Mi", "1Gi")
                - `gpu`: Number of GPU devices (e.g., "1")

                New resource types can be added without API changes.
                 Example: {'cpu': '500m', 'memory': '512Mi', 'gpu': '1'}.
            entrypoint (list[str]): The command to execute as the sandbox's entry process (required).

                Explicitly specifies the user's expected main process, allowing the sandbox management
                service to reliably inject control processes before executing this command.

                Format: [executable, arg1, arg2, ...]

                Examples:
                - ["python", "/app/main.py"]
                - ["/bin/bash"]
                - ["java", "-jar", "/app/app.jar"]
                - ["node", "server.js"]
                 Example: ['python', '/app/main.py'].
            env (CreateSandboxRequestEnv | Unset): Environment variables to inject into the sandbox runtime. Example:
                {'API_KEY': 'secret-key', 'DEBUG': 'true', 'LOG_LEVEL': 'info'}.
            metadata (CreateSandboxRequestMetadata | Unset): Custom key-value metadata for management, filtering, and
                tagging.
                Use "name" key for a human-readable identifier.
                 Example: {'name': 'Data Processing Sandbox', 'project': 'data-processing', 'team': 'ml', 'environment':
                'staging'}.
            network_policy (NetworkPolicy | Unset): Egress network policy matching the sidecar `/policy` request body.
                If `defaultAction` is omitted, the sidecar defaults to "deny"; passing an empty
                object or null results in allow-all behavior at startup.
            volumes (list[Volume] | Unset): Storage mounts for the sandbox. Each volume entry specifies a named backend-
                specific
                storage source and common mount settings. Exactly one backend type must be specified
                per volume entry.
            extensions (CreateSandboxRequestExtensions | Unset): Opaque container for provider-specific or transient
                parameters not supported by the core API.

                **Note**: This field is reserved for internal features, experimental flags, or temporary behaviors. Standard
                parameters should be proposed as core API fields.

                **Best Practices**:
                - **Namespacing**: Use prefixed keys (e.g., `storage.id`) to prevent collisions.
                - **Pass-through**: SDKs and middleware must treat this object as opaque and pass it through transparently.
    """

    image: ImageSpec
    timeout: int
    resource_limits: ResourceLimits
    entrypoint: list[str]
    env: CreateSandboxRequestEnv | Unset = UNSET
    metadata: CreateSandboxRequestMetadata | Unset = UNSET
    network_policy: NetworkPolicy | Unset = UNSET
    volumes: list[Volume] | Unset = UNSET
    extensions: CreateSandboxRequestExtensions | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        image = self.image.to_dict()

        timeout = self.timeout

        resource_limits = self.resource_limits.to_dict()

        entrypoint = self.entrypoint

        env: dict[str, Any] | Unset = UNSET
        if not isinstance(self.env, Unset):
            env = self.env.to_dict()

        metadata: dict[str, Any] | Unset = UNSET
        if not isinstance(self.metadata, Unset):
            metadata = self.metadata.to_dict()

        network_policy: dict[str, Any] | Unset = UNSET
        if not isinstance(self.network_policy, Unset):
            network_policy = self.network_policy.to_dict()

        volumes: list[dict[str, Any]] | Unset = UNSET
        if not isinstance(self.volumes, Unset):
            volumes = []
            for volumes_item_data in self.volumes:
                volumes_item = volumes_item_data.to_dict()
                volumes.append(volumes_item)

        extensions: dict[str, Any] | Unset = UNSET
        if not isinstance(self.extensions, Unset):
            extensions = self.extensions.to_dict()

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update(
            {
                "image": image,
                "timeout": timeout,
                "resourceLimits": resource_limits,
                "entrypoint": entrypoint,
            }
        )
        if env is not UNSET:
            field_dict["env"] = env
        if metadata is not UNSET:
            field_dict["metadata"] = metadata
        if network_policy is not UNSET:
            field_dict["networkPolicy"] = network_policy
        if volumes is not UNSET:
            field_dict["volumes"] = volumes
        if extensions is not UNSET:
            field_dict["extensions"] = extensions

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.create_sandbox_request_env import CreateSandboxRequestEnv
        from ..models.create_sandbox_request_extensions import CreateSandboxRequestExtensions
        from ..models.create_sandbox_request_metadata import CreateSandboxRequestMetadata
        from ..models.image_spec import ImageSpec
        from ..models.network_policy import NetworkPolicy
        from ..models.resource_limits import ResourceLimits
        from ..models.volume import Volume

        d = dict(src_dict)
        image = ImageSpec.from_dict(d.pop("image"))

        timeout = d.pop("timeout")

        resource_limits = ResourceLimits.from_dict(d.pop("resourceLimits"))

        entrypoint = cast(list[str], d.pop("entrypoint"))

        _env = d.pop("env", UNSET)
        env: CreateSandboxRequestEnv | Unset
        if isinstance(_env, Unset):
            env = UNSET
        else:
            env = CreateSandboxRequestEnv.from_dict(_env)

        _metadata = d.pop("metadata", UNSET)
        metadata: CreateSandboxRequestMetadata | Unset
        if isinstance(_metadata, Unset):
            metadata = UNSET
        else:
            metadata = CreateSandboxRequestMetadata.from_dict(_metadata)

        _network_policy = d.pop("networkPolicy", UNSET)
        network_policy: NetworkPolicy | Unset
        if isinstance(_network_policy, Unset):
            network_policy = UNSET
        else:
            network_policy = NetworkPolicy.from_dict(_network_policy)

        _volumes = d.pop("volumes", UNSET)
        volumes: list[Volume] | Unset = UNSET
        if _volumes is not UNSET:
            volumes = []
            for volumes_item_data in _volumes:
                volumes_item = Volume.from_dict(volumes_item_data)

                volumes.append(volumes_item)

        _extensions = d.pop("extensions", UNSET)
        extensions: CreateSandboxRequestExtensions | Unset
        if isinstance(_extensions, Unset):
            extensions = UNSET
        else:
            extensions = CreateSandboxRequestExtensions.from_dict(_extensions)

        create_sandbox_request = cls(
            image=image,
            timeout=timeout,
            resource_limits=resource_limits,
            entrypoint=entrypoint,
            env=env,
            metadata=metadata,
            network_policy=network_policy,
            volumes=volumes,
            extensions=extensions,
        )

        create_sandbox_request.additional_properties = d
        return create_sandbox_request

    @property
    def additional_keys(self) -> list[str]:
        return list(self.additional_properties.keys())

    def __getitem__(self, key: str) -> Any:
        return self.additional_properties[key]

    def __setitem__(self, key: str, value: Any) -> None:
        self.additional_properties[key] = value

    def __delitem__(self, key: str) -> None:
        del self.additional_properties[key]

    def __contains__(self, key: str) -> bool:
        return key in self.additional_properties

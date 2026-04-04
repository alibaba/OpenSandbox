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

"""Pool CRD CRUD via ``K8sClient`` (rate limits; get may use informer cache)."""

import logging
from typing import Any, Dict, Optional

from fastapi import HTTPException, status
from kubernetes.client import ApiException
from pydantic import ValidationError

from opensandbox_server.api.schema import (
    CreatePoolRequest,
    ListPoolsResponse,
    PoolCapacitySpec,
    PoolResponse,
    PoolStatus,
    UpdatePoolRequest,
)
from opensandbox_server.services.constants import SandboxErrorCodes
from opensandbox_server.services.k8s.client import K8sClient
from opensandbox_server.services.k8s.kubernetes_api_exception import (
    http_status_from_kubernetes_api_exception,
    kubernetes_api_exception_message,
)

logger = logging.getLogger(__name__)

_GROUP = "sandbox.opensandbox.io"
_VERSION = "v1alpha1"
_PLURAL = "pools"


def _k8s_pool_detail(message: str) -> Dict[str, str]:
    return {"code": SandboxErrorCodes.K8S_POOL_API_ERROR, "message": message}


class PoolService:
    def __init__(self, k8s_client: K8sClient, namespace: str) -> None:
        self._k8s = k8s_client
        self._namespace = namespace

    def _raise_for_kubernetes_api_exception(
        self,
        exc: ApiException,
        *,
        operation: str,
        pool_name: Optional[str],
    ) -> None:
        http_status = http_status_from_kubernetes_api_exception(exc)
        message = kubernetes_api_exception_message(exc) or (
            f"Kubernetes API request failed (HTTP {http_status})"
        )
        suffix = f" pool={pool_name}" if pool_name else ""
        if http_status >= 500:
            logger.error(
                f"Kubernetes API error op={operation}{suffix} status={exc.status} "
                f"message={message!r} exc={exc!r}"
            )
        else:
            logger.warning(
                f"Kubernetes API rejection op={operation}{suffix} status={exc.status} message={message!r}"
            )
        raise HTTPException(status_code=http_status, detail=_k8s_pool_detail(message)) from exc

    def _raise_pool_internal_error(
        self, operation: str, pool_name: Optional[str], exc: Exception
    ) -> None:
        if operation == "list":
            logger.error(f"Unexpected error listing pools: {exc}")
            msg = f"Failed to list pools: {exc}"
        else:
            logger.error(f"Unexpected error {operation} pool {pool_name}: {exc}")
            msg = f"Failed to {operation} pool: {exc}"
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=_k8s_pool_detail(msg),
        ) from exc

    def _named_pool_api_exception(
        self, exc: ApiException, pool_name: str, operation: str
    ) -> None:
        if exc.status == 404:
            msg = kubernetes_api_exception_message(exc) or f"Pool '{pool_name}' not found."
            raise HTTPException(
                status_code=status.HTTP_404_NOT_FOUND,
                detail={
                    "code": SandboxErrorCodes.K8S_POOL_NOT_FOUND,
                    "message": msg,
                },
            ) from exc
        self._raise_for_kubernetes_api_exception(
            exc, operation=operation, pool_name=pool_name
        )

    def _capacity_spec_from_raw(self, pool_name: str, capacity: Any) -> PoolCapacitySpec:
        if capacity is None:
            logger.warning(f"Pool {pool_name or '?'}: missing capacitySpec, using zeros")
            data: Dict[str, Any] = {}
        elif not isinstance(capacity, dict):
            logger.error(f"Pool {pool_name or '?'}: capacitySpec not an object")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=_k8s_pool_detail(
                    "Invalid Pool capacitySpec returned from Kubernetes (expected object)."
                ),
            )
        else:
            data = capacity
            if not data:
                logger.warning(f"Pool {pool_name or '?'}: empty capacitySpec, using zeros")

        payload = {
            "bufferMax": data.get("bufferMax", 0),
            "bufferMin": data.get("bufferMin", 0),
            "poolMax": data.get("poolMax", 0),
            "poolMin": data.get("poolMin", 0),
        }
        try:
            return PoolCapacitySpec.model_validate(payload)
        except ValidationError as e:
            logger.error(f"Pool {pool_name or '?'}: invalid capacitySpec: {e}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=_k8s_pool_detail(
                    "Invalid Pool capacitySpec returned from Kubernetes (types or ranges)."
                ),
            ) from e

    def _pool_status_from_raw(self, pool_name: str, raw_status: Any) -> Optional[PoolStatus]:
        if raw_status is None:
            return None
        if not isinstance(raw_status, dict):
            logger.warning(
                f"Pool {pool_name or '?'}: status not an object, ignoring"
            )
            return None
        if not raw_status:
            return None
        payload = {
            "total": raw_status.get("total", 0),
            "allocated": raw_status.get("allocated", 0),
            "available": raw_status.get("available", 0),
            "revision": raw_status.get("revision", ""),
        }
        try:
            return PoolStatus.model_validate(payload)
        except ValidationError as e:
            logger.error(f"Pool {pool_name or '?'}: invalid status: {e}")
            raise HTTPException(
                status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
                detail=_k8s_pool_detail(
                    "Invalid Pool status returned from Kubernetes (types or ranges)."
                ),
            ) from e

    def _build_pool_manifest(
        self,
        name: str,
        namespace: str,
        template: Dict[str, Any],
        capacity_spec: PoolCapacitySpec,
    ) -> Dict[str, Any]:
        return {
            "apiVersion": f"{_GROUP}/{_VERSION}",
            "kind": "Pool",
            "metadata": {"name": name, "namespace": namespace},
            "spec": {
                "template": template,
                "capacitySpec": {
                    "bufferMax": capacity_spec.buffer_max,
                    "bufferMin": capacity_spec.buffer_min,
                    "poolMax": capacity_spec.pool_max,
                    "poolMin": capacity_spec.pool_min,
                },
            },
        }

    def _pool_from_raw(self, raw: Dict[str, Any]) -> PoolResponse:
        metadata = raw.get("metadata")
        if not isinstance(metadata, dict):
            if metadata is not None:
                logger.warning(f"Pool metadata not a dict ({type(metadata).__name__}), using {{}}")
            metadata = {}
        pool_name = metadata.get("name") or ""
        if not pool_name:
            logger.warning("Pool missing metadata.name")

        spec = raw.get("spec")
        if not isinstance(spec, dict):
            if spec is not None:
                logger.warning(f"Pool {pool_name or '?'}: spec not a dict, using {{}}")
            spec = {}

        return PoolResponse(
            name=pool_name,
            capacitySpec=self._capacity_spec_from_raw(pool_name, spec.get("capacitySpec")),
            status=self._pool_status_from_raw(pool_name, raw.get("status")),
            createdAt=metadata.get("creationTimestamp"),
        )

    def create_pool(self, request: CreatePoolRequest) -> PoolResponse:
        manifest = self._build_pool_manifest(
            request.name, self._namespace, request.template, request.capacity_spec
        )
        try:
            created = self._k8s.create_custom_object(
                group=_GROUP,
                version=_VERSION,
                namespace=self._namespace,
                plural=_PLURAL,
                body=manifest,
            )
            logger.info(f"Created pool name={request.name} namespace={self._namespace}")
            return self._pool_from_raw(created)
        except HTTPException:
            raise
        except ApiException as e:
            if e.status == 409:
                msg = kubernetes_api_exception_message(e) or f"Pool '{request.name}' already exists."
                raise HTTPException(
                    status_code=status.HTTP_409_CONFLICT,
                    detail={
                        "code": SandboxErrorCodes.K8S_POOL_ALREADY_EXISTS,
                        "message": msg,
                    },
                ) from e
            self._raise_for_kubernetes_api_exception(
                e, operation="create", pool_name=request.name
            )
        except Exception as e:
            self._raise_pool_internal_error("create", request.name, e)

    def get_pool(self, pool_name: str) -> PoolResponse:
        try:
            raw = self._k8s.get_custom_object(
                group=_GROUP,
                version=_VERSION,
                namespace=self._namespace,
                plural=_PLURAL,
                name=pool_name,
            )
            if raw is None:
                raise HTTPException(
                    status_code=status.HTTP_404_NOT_FOUND,
                    detail={
                        "code": SandboxErrorCodes.K8S_POOL_NOT_FOUND,
                        "message": f"Pool '{pool_name}' not found.",
                    },
                )
            return self._pool_from_raw(raw)
        except HTTPException:
            raise
        except ApiException as e:
            self._raise_for_kubernetes_api_exception(e, operation="get", pool_name=pool_name)
        except Exception as e:
            self._raise_pool_internal_error("get", pool_name, e)

    def list_pools(self) -> ListPoolsResponse:
        try:
            raw_items = self._k8s.list_custom_objects(
                group=_GROUP,
                version=_VERSION,
                namespace=self._namespace,
                plural=_PLURAL,
                not_found_returns_empty=False,
            )
            return ListPoolsResponse(
                items=[self._pool_from_raw(item) for item in raw_items]
            )
        except HTTPException:
            raise
        except ApiException as e:
            if e.status == 404:
                logger.warning(
                    f"Pool list 404 (CRD/path?): {_GROUP}/{_VERSION} {_PLURAL} ns={self._namespace}"
                )
                raise HTTPException(
                    status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
                    detail={
                        "code": SandboxErrorCodes.K8S_POOL_LIST_UNAVAILABLE,
                        "message": (
                            "Cannot list Pool resources: Kubernetes returned 404 for the Pool "
                            f"API path. Check Pool CRD and group/version ({_GROUP}/{_VERSION})."
                        ),
                    },
                ) from e
            self._raise_for_kubernetes_api_exception(e, operation="list", pool_name=None)
        except Exception as e:
            self._raise_pool_internal_error("list", None, e)

    def update_pool(self, pool_name: str, request: UpdatePoolRequest) -> PoolResponse:
        patch_body = {
            "spec": {
                "capacitySpec": {
                    "bufferMax": request.capacity_spec.buffer_max,
                    "bufferMin": request.capacity_spec.buffer_min,
                    "poolMax": request.capacity_spec.pool_max,
                    "poolMin": request.capacity_spec.pool_min,
                }
            }
        }
        try:
            updated = self._k8s.patch_custom_object(
                group=_GROUP,
                version=_VERSION,
                namespace=self._namespace,
                plural=_PLURAL,
                name=pool_name,
                body=patch_body,
            )
            logger.info(f"Updated pool capacity name={pool_name}")
            return self._pool_from_raw(updated)
        except HTTPException:
            raise
        except ApiException as e:
            self._named_pool_api_exception(e, pool_name, "update")
        except Exception as e:
            self._raise_pool_internal_error("update", pool_name, e)

    def delete_pool(self, pool_name: str) -> None:
        try:
            self._k8s.delete_custom_object(
                group=_GROUP,
                version=_VERSION,
                namespace=self._namespace,
                plural=_PLURAL,
                name=pool_name,
                grace_period_seconds=0,
            )
            logger.info(f"Deleted pool name={pool_name} namespace={self._namespace}")
        except HTTPException:
            raise
        except ApiException as e:
            self._named_pool_api_exception(e, pool_name, "delete")
        except Exception as e:
            self._raise_pool_internal_error("delete", pool_name, e)

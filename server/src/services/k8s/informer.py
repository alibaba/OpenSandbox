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

"""Lightweight informer-style cache for namespaced custom resources."""

import logging
import threading
from typing import Any, Dict, Optional

from kubernetes import watch
from kubernetes.client import ApiException, CustomObjectsApi

logger = logging.getLogger(__name__)


class WorkloadInformer:
    """Maintain an in-memory cache of a namespaced custom resource via watch."""

    def __init__(
        self,
        custom_api: CustomObjectsApi,
        group: str,
        version: str,
        plural: str,
        namespace: str,
        resync_period_seconds: int = 300,
        watch_timeout_seconds: int = 60,
        enable_watch: bool = True,
    ):
        self.custom_api = custom_api
        self.group = group
        self.version = version
        self.plural = plural
        self.namespace = namespace
        self.resync_period_seconds = resync_period_seconds
        self.watch_timeout_seconds = watch_timeout_seconds
        self.enable_watch = enable_watch

        self._cache: Dict[str, Dict[str, Any]] = {}
        self._lock = threading.RLock()
        self._resource_version: Optional[str] = None
        self._has_synced = False
        self._stop_event = threading.Event()
        self._thread: Optional[threading.Thread] = None

    @property
    def has_synced(self) -> bool:
        """Return True once an initial list has completed."""
        return self._has_synced

    def start(self) -> None:
        """Start the background watch thread if not already running."""
        if self._thread and self._thread.is_alive():
            return

        self._thread = threading.Thread(
            target=self._run,
            name=f"workload-informer-{self.plural}-{self.namespace}",
            daemon=True,
        )
        self._thread.start()

    def stop(self) -> None:
        """Stop the background watch thread."""
        self._stop_event.set()

    def get(self, name: str) -> Optional[Dict[str, Any]]:
        """Return cached object by name, if present."""
        with self._lock:
            obj = self._cache.get(name)
            if obj:
                return obj
        return None

    def update_cache(self, obj: Dict[str, Any]) -> None:
        """Upsert a single object into the cache."""
        metadata = obj.get("metadata", {})
        name = metadata.get("name")
        if not name:
            return

        with self._lock:
            self._cache[name] = obj
            if metadata.get("resourceVersion"):
                self._resource_version = metadata["resourceVersion"]

    def _run(self) -> None:
        backoff = 1.0
        while not self._stop_event.is_set():
            try:
                if not self._has_synced:
                    self._full_resync()
                    backoff = 1.0

                if not self.enable_watch:
                    self._stop_event.wait(self.resync_period_seconds)
                    continue

                self._run_watch_loop()
                backoff = 1.0
            except ApiException as exc:
                if exc.status == 410:
                    # Resource version too old; force a fresh list on next loop.
                    self._resource_version = None
                    self._has_synced = False
                else:
                    logger.warning(
                        "Informer watch error for %s: %s", self.plural, exc, exc_info=True
                    )
                    self._has_synced = False
                    self._stop_event.wait(min(backoff, 30.0))
                    backoff = min(backoff * 2, 30.0)
            except Exception as exc:  # pragma: no cover - defensive
                logger.warning(
                    "Unexpected informer error for %s: %s", self.plural, exc, exc_info=True
                )
                self._has_synced = False
                self._stop_event.wait(min(backoff, 30.0))
                backoff = min(backoff * 2, 30.0)

    def _full_resync(self) -> None:
        """Perform a full list to refresh the cache."""
        resp = self.custom_api.list_namespaced_custom_object(
            group=self.group,
            version=self.version,
            namespace=self.namespace,
            plural=self.plural,
        )

        # list response is a dict for CustomObjectsApi
        items = resp.get("items", []) if isinstance(resp, dict) else []
        metadata = resp.get("metadata", {}) if isinstance(resp, dict) else {}
        resource_version = metadata.get("resourceVersion")

        # Build new cache outside the lock to avoid blocking readers
        new_cache: Dict[str, Dict[str, Any]] = {}
        for item in items:
            name = item.get("metadata", {}).get("name")
            if name:
                new_cache[name] = item

        with self._lock:
            self._cache = new_cache
            if resource_version:
                self._resource_version = resource_version
            self._has_synced = True

    def _run_watch_loop(self) -> None:
        """Stream watch events to keep the cache fresh."""
        w = watch.Watch()
        try:
            for event in w.stream(
                self.custom_api.list_namespaced_custom_object,
                group=self.group,
                version=self.version,
                namespace=self.namespace,
                plural=self.plural,
                resource_version=self._resource_version,
                timeout_seconds=self.watch_timeout_seconds,
            ):
                if self._stop_event.is_set():
                    break
                self._handle_event(event)
        finally:
            w.stop()

    def _handle_event(self, event: Dict[str, Any]) -> None:
        obj = event.get("object")
        if obj is None:
            return

        if not isinstance(obj, dict):
            try:
                obj = obj.to_dict()
            except Exception:
                return

        metadata = obj.get("metadata", {})
        name = metadata.get("name")
        if not name:
            return

        event_type = event.get("type")
        with self._lock:
            if event_type == "DELETED":
                self._cache.pop(name, None)
            else:
                self._cache[name] = obj
            if metadata.get("resourceVersion"):
                self._resource_version = metadata["resourceVersion"]

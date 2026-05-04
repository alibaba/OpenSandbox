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

"""Hot-reload tenant registry backed by tenants.toml and fsnotify."""

from __future__ import annotations

import logging
import threading
from pathlib import Path

from watchfiles import watch

from .models import TenantEntry

logger = logging.getLogger(__name__)

try:
    import tomllib
except ModuleNotFoundError:
    import tomli as tomllib  # type: ignore[import, no-redef]


class TenantLoader:
    """Thread-safe in-memory tenant registry. Reloads on file change."""

    def __init__(self, path: str | Path) -> None:
        self._path = Path(path)
        self._entries: dict[str, TenantEntry] = {}
        self._lock = threading.Lock()
        self._stop_event = threading.Event()

        if self._path.exists():
            self._reload()
            self._start_watcher()

    def lookup(self, api_key: str) -> TenantEntry | None:
        with self._lock:
            return self._entries.get(api_key)

    def stop(self) -> None:
        self._stop_event.set()

    @property
    def tenant_count(self) -> int:
        with self._lock:
            return len({e.name for e in self._entries.values()})

    def _reload(self) -> None:
        data = tomllib.loads(self._path.read_text())
        new_entries: dict[str, TenantEntry] = {}
        for raw in data.get("tenants", []):
            entry = TenantEntry(**raw)
            for k in entry.api_keys:
                if k in new_entries:
                    raise ValueError(
                        f"Duplicate api_key '{k}' across tenants "
                        f"'{new_entries[k].name}' and '{entry.name}'"
                    )
                new_entries[k] = entry
        with self._lock:
            self._entries = new_entries
        logger.info(f"Reloaded {len(new_entries)} tenant(s) from {self._path}.")

    def _start_watcher(self) -> None:
        parent = self._path.parent
        target = self._path.resolve()

        def _watch() -> None:
            try:
                for changes in watch(parent, stop_event=self._stop_event):
                    for _change_type, changed_path in changes:
                        if Path(changed_path).resolve() == target:
                            try:
                                self._reload()
                            except Exception:
                                logger.exception(
                                    f"Failed to reload {self._path}; keeping old entries."
                                )
            except Exception:
                logger.exception("Tenant file watcher exited unexpectedly.")

        t = threading.Thread(target=_watch, daemon=True)
        t.start()
        logger.info(f"Started fsnotify watcher on {parent}.")

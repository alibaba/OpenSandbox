# Copyright 2026 The OpenSandbox Authors
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

"""``opensandbox.server.idlep.*`` instruments; noop without OTEL, never raises."""

from __future__ import annotations

import logging
from typing import Optional

from opentelemetry import metrics

logger = logging.getLogger(__name__)

PAUSE_REQUESTS_COUNTER = "opensandbox.server.idlep.pause.requests"
PAUSE_LATENCY_HISTOGRAM = "opensandbox.server.idlep.pause.latency"
PROBE_OUTCOME_COUNTER = "opensandbox.server.idlep.probe.outcome"

_meter: Optional[metrics.Meter] = None
_pause_requests = None
_pause_latency = None
_probe_outcomes = None


def _counter():
    global _meter, _pause_requests
    if _pause_requests is None:
        try:
            meter = _meter or metrics.get_meter("opensandbox.server")
            _meter = meter
            _pause_requests = meter.create_counter(
                name=PAUSE_REQUESTS_COUNTER,
                unit="{pause}",
                description="Idle-pause attempts by outcome (accepted / gated / failed).",
            )
        except Exception:  # pragma: no cover - defensive; never break a sweep
            logger.exception("Failed to create idlep instruments")
            return None
    return _pause_requests


def _histogram():
    global _meter, _pause_latency
    if _pause_latency is None:
        try:
            meter = _meter or metrics.get_meter("opensandbox.server")
            _meter = meter
            _pause_latency = meter.create_histogram(
                name=PAUSE_LATENCY_HISTOGRAM,
                unit="s",
                description=(
                    "PauseSandbox accepted -> observed Paused (checkpoint duration)."
                ),
                explicit_bucket_boundaries_advisory=(
                    0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300,
                ),
            )
        except Exception:  # pragma: no cover - defensive; never break a sweep
            logger.exception("Failed to create idlep instruments")
            return None
    return _pause_latency


def record_pause_request(*, outcome: str) -> None:
    """Count one sweeper pause attempt. ``outcome``: accepted/gated/failed."""
    counter = _counter()
    if counter is None:
        return
    try:
        counter.add(1, attributes={"outcome": outcome})
    except Exception:
        logger.exception("Failed to record idlep.pause.requests")


def _probe_counter():
    global _meter, _probe_outcomes
    if _probe_outcomes is None:
        try:
            meter = _meter or metrics.get_meter("opensandbox.server")
            _meter = meter
            _probe_outcomes = meter.create_counter(
                name=PROBE_OUTCOME_COUNTER,
                unit="{probe}",
                description=(
                    "Activity-probe results (active / not_active / error); "
                    "sustained error means a broken probe endpoint on an "
                    "opted-in workload."
                ),
            )
        except Exception:  # pragma: no cover - defensive; never break a sweep
            logger.exception("Failed to create idlep instruments")
            return None
    return _probe_outcomes


def record_probe_outcome(*, outcome: str) -> None:
    """Count one activity-probe verdict. ``outcome``: active/not_active/error."""
    counter = _probe_counter()
    if counter is None:
        return
    try:
        counter.add(1, attributes={"outcome": outcome})
    except Exception:
        logger.exception("Failed to record idlep.probe.outcome")


def record_pause_latency(*, seconds: float) -> None:
    """Record one accepted pause's accepted->Paused duration."""
    histogram = _histogram()
    if histogram is None:
        return
    try:
        histogram.record(max(0.0, seconds))
    except Exception:
        logger.exception("Failed to record idlep.pause.latency")

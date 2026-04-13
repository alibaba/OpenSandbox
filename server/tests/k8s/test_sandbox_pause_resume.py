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
Unit tests for pause_sandbox and resume_sandbox functionality.

Tests cover KubernetesSandboxService pause and resume operations including
SandboxSnapshot CR creation, state validation, and error handling.
"""

import pytest
from datetime import datetime, timezone

from fastapi import HTTPException

from opensandbox_server.services.constants import SandboxErrorCodes


class TestPauseSandbox:
    """pause_sandbox method tests"""

    def test_pause_sandbox_success(self, k8s_service):
        """
        Test case: Successfully pause a running sandbox

        Purpose: Verify that SandboxSnapshot CR is created with pause config from server config.
        """
        sandbox_id = "test-sandbox-123"

        # Mock BatchSandbox in Running state (pausePolicy removed from CRD)
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {
                "name": sandbox_id,
                "namespace": "test-namespace",
            },
            "spec": {
                "replicas": 1,
                "template": {"spec": {"containers": [{"name": "sandbox", "image": "python:3.11"}]}},
            },
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "POD_READY_WITH_IP",
            "message": "Pod is ready",
            "last_transition_at": datetime.now(timezone.utc),
        }

        # Mock no existing snapshot
        k8s_service.snapshot_provider.get_snapshot.return_value = None

        # Mock snapshot creation
        k8s_service.snapshot_provider.create_snapshot.return_value = {
            "metadata": {"name": sandbox_id}
        }

        k8s_service.pause_sandbox(sandbox_id)

        # Verify SandboxSnapshot CR was created
        k8s_service.snapshot_provider.create_snapshot.assert_called_once()
        call_args = k8s_service.snapshot_provider.create_snapshot.call_args
        assert call_args[0][0] == k8s_service.namespace

        snapshot_cr = call_args[0][1]
        assert snapshot_cr["metadata"]["name"] == sandbox_id
        assert snapshot_cr["metadata"]["labels"]["sandbox.opensandbox.io/sandbox-id"] == sandbox_id
        assert snapshot_cr["spec"]["sandboxId"] == sandbox_id
        assert snapshot_cr["spec"]["sourceBatchSandboxName"] == sandbox_id
        assert snapshot_cr["spec"]["action"] == "Pause"
        # Verify config values are written to spec
        assert snapshot_cr["spec"]["snapshotType"] == "Rootfs"
        assert snapshot_cr["spec"]["snapshotRegistry"] == "registry.example.com/snapshots"
        assert snapshot_cr["spec"]["snapshotPushSecret"] == "push-secret"
        assert snapshot_cr["spec"]["resumeImagePullSecret"] == "pull-secret"

    def test_pause_sandbox_with_secrets(self, k8s_service):
        """
        Test case: Pause sandbox with push/pull secrets from server config

        Purpose: Verify secrets from config are included in SandboxSnapshot spec
        """
        sandbox_id = "test-sandbox-secrets"

        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {
                "name": sandbox_id,
                "namespace": "test-namespace",
            },
            "spec": {
                "replicas": 1,
                "template": {"spec": {"containers": [{"name": "sandbox", "image": "python:3.11"}]}},
            },
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "POD_READY_WITH_IP",
            "message": "Pod is ready",
            "last_transition_at": datetime.now(timezone.utc),
        }

        k8s_service.snapshot_provider.get_snapshot.return_value = None
        k8s_service.snapshot_provider.create_snapshot.return_value = {
            "metadata": {"name": sandbox_id}
        }

        k8s_service.pause_sandbox(sandbox_id)

        call_args = k8s_service.snapshot_provider.create_snapshot.call_args
        snapshot_cr = call_args[0][1]
        assert snapshot_cr["spec"]["sandboxId"] == sandbox_id
        assert snapshot_cr["spec"]["sourceBatchSandboxName"] == sandbox_id
        assert snapshot_cr["spec"]["action"] == "Pause"
        # Secrets from config are written to spec
        assert snapshot_cr["spec"]["snapshotPushSecret"] == "push-secret"
        assert snapshot_cr["spec"]["resumeImagePullSecret"] == "pull-secret"

    def test_pause_sandbox_not_found(self, k8s_service):
        """
        Test case: Pause sandbox that does not exist

        Purpose: Verify that 404 is returned when sandbox is not found
        """
        sandbox_id = "nonexistent-sandbox"

        # Mock no BatchSandbox
        k8s_service.workload_provider.get_workload.return_value = None
        # Mock no snapshot either
        k8s_service.snapshot_provider.get_snapshot.return_value = None

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.pause_sandbox(sandbox_id)

        assert exc_info.value.status_code == 404
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND

    def test_pause_sandbox_already_paused(self, k8s_service):
        """
        Test case: Pause sandbox that is already paused (no BatchSandbox but has Snapshot)

        Purpose: Verify that 409 is returned when sandbox is already paused,
        not 404. This ensures idempotent pause flows work correctly.
        """
        sandbox_id = "already-paused-sandbox"

        # Mock no BatchSandbox (deleted by controller after pause)
        k8s_service.workload_provider.get_workload.return_value = None
        # Mock existing snapshot in Ready phase (pause completed)
        k8s_service.snapshot_provider.get_snapshot.return_value = {
            "metadata": {"name": sandbox_id},
            "status": {"phase": "Ready"},
        }

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.pause_sandbox(sandbox_id)

        assert exc_info.value.status_code == 409
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_STATE
        assert "already paused" in exc_info.value.detail["message"]

    def test_pause_sandbox_invalid_state(self, k8s_service):
        """
        Test case: Pause sandbox that is not in Running state

        Purpose: Verify that 409 is returned when sandbox is not in Running state
        """
        sandbox_id = "pending-sandbox"

        # Mock BatchSandbox in Pending state (pausePolicy removed from CRD)
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "replicas": 1,
            },
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Pending",
            "reason": "POD_SCHEDULED",
            "message": "Pod is pending",
            "last_transition_at": datetime.now(timezone.utc),
        }

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.pause_sandbox(sandbox_id)

        assert exc_info.value.status_code == 409
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_STATE

    def test_pause_sandbox_no_pause_policy(self, k8s_service):
        """
        Test case: Pause sandbox when server config has no pause config

        Purpose: Verify that 400 is returned when pause.snapshot_registry is not configured
        """
        sandbox_id = "sandbox-without-pause-config"

        # Set pause config to None to simulate missing config
        k8s_service.app_config.pause = None

        # Mock BatchSandbox in Running state
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "replicas": 1,
            },
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "POD_READY_WITH_IP",
            "message": "Pod is running",
            "last_transition_at": datetime.now(timezone.utc),
        }

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.pause_sandbox(sandbox_id)

        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.PAUSE_POLICY_NOT_CONFIGURED

    def test_pause_sandbox_snapshot_committing_conflict(self, k8s_service):
        """
        Test case: Pause sandbox when snapshot is in Committing phase

        Purpose: Verify that 409 is returned via phase-based conflict detection
        """
        sandbox_id = "sandbox-with-snapshot-in-progress"

        # Mock BatchSandbox in Running state (pausePolicy removed from CRD)
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "replicas": 1,
            },
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "POD_READY_WITH_IP",
            "message": "Pod is running",
            "last_transition_at": datetime.now(timezone.utc),
        }

        # Mock existing snapshot in Committing phase
        k8s_service.snapshot_provider.get_snapshot.return_value = {
            "metadata": {"name": sandbox_id},
            "status": {"phase": "Committing"},
        }

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.pause_sandbox(sandbox_id)

        assert exc_info.value.status_code == 409
        assert exc_info.value.detail["code"] == SandboxErrorCodes.SNAPSHOT_IN_PROGRESS

    def test_pause_sandbox_snapshot_pending_conflict(self, k8s_service):
        """
        Test case: Pause sandbox when snapshot is in Pending phase

        Purpose: Verify that 409 is returned via phase-based conflict detection
        for Pending phase as well.
        """
        sandbox_id = "sandbox-with-pending-snapshot"

        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "replicas": 1,
            },
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "POD_READY_WITH_IP",
            "message": "Pod is running",
            "last_transition_at": datetime.now(timezone.utc),
        }

        # Mock existing snapshot in Pending phase
        k8s_service.snapshot_provider.get_snapshot.return_value = {
            "metadata": {"name": sandbox_id},
            "status": {"phase": "Pending"},
        }

        with pytest.raises(HTTPException) as exc_info:
            k8s_service.pause_sandbox(sandbox_id)

        assert exc_info.value.status_code == 409
        assert exc_info.value.detail["code"] == SandboxErrorCodes.SNAPSHOT_IN_PROGRESS

    def test_pause_sandbox_re_pause(self, k8s_service):
        """
        Test case: Re-pause a sandbox that already has a Ready snapshot

        Purpose: Verify that patch_snapshot_spec is called with
        sourceBatchSandboxName and action="Pause".
        """
        sandbox_id = "sandbox-re-pause"

        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {"name": sandbox_id, "namespace": "test-namespace"},
            "spec": {
                "replicas": 1,
                "template": {"spec": {"containers": [{"name": "sandbox", "image": "python:3.11"}]}},
            },
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "POD_READY_WITH_IP",
            "message": "Pod is ready",
            "last_transition_at": datetime.now(timezone.utc),
        }

        # Mock existing snapshot in Ready phase (previous pause completed)
        k8s_service.snapshot_provider.get_snapshot.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "sandboxId": sandbox_id,
                "sourceBatchSandboxName": sandbox_id,
                "action": "Pause",
            },
            "status": {"phase": "Ready"},
        }

        k8s_service.snapshot_provider.patch_snapshot_spec.return_value = {
            "metadata": {"name": sandbox_id}
        }

        k8s_service.pause_sandbox(sandbox_id)

        # Verify patch_snapshot_spec was called (not create_snapshot)
        k8s_service.snapshot_provider.patch_snapshot_spec.assert_called_once()
        call_args = k8s_service.snapshot_provider.patch_snapshot_spec.call_args
        assert call_args[1]["snapshot_name"] == sandbox_id
        spec_patch = call_args[1]["spec_patch"]
        assert spec_patch["sourceBatchSandboxName"] == sandbox_id
        assert spec_patch["action"] == "Pause"

        # create_snapshot should NOT be called
        k8s_service.snapshot_provider.create_snapshot.assert_not_called()

    def test_pause_sandbox_unsupported_replicas(self, k8s_service):
        """
        Test case: Pause sandbox with replicas != 1

        Purpose: Verify that 400 is returned for unsupported replicas count
        """
        sandbox_id = "multi-replica-sandbox"

        # Mock BatchSandbox with replicas=2
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "replicas": 2,  # Unsupported
            },
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "POD_READY_WITH_IP",
            "message": "Pod is running",
            "last_transition_at": datetime.now(timezone.utc),
        }

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.pause_sandbox(sandbox_id)

        assert exc_info.value.status_code == 400
        assert exc_info.value.detail["code"] == SandboxErrorCodes.UNSUPPORTED_REPLICAS


class TestResumeSandbox:
    """resume_sandbox method tests"""

    def test_resume_sandbox_success(self, k8s_service):
        """
        Test case: Resume sandbox by setting action="Resume"

        Purpose: Verify that patch_snapshot_spec is called with
        action="Resume" to trigger controller-driven resume.
        """
        sandbox_id = "paused-sandbox-multi"

        # Mock Snapshot in Ready phase
        k8s_service.snapshot_provider.get_snapshot.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "sandboxId": sandbox_id,
                "sourceBatchSandboxName": sandbox_id,
                "action": "Pause",
            },
            "status": {"phase": "Ready"},
        }

        # Mock no existing BatchSandbox
        k8s_service.workload_provider.get_workload.return_value = None

        # Mock patch success
        k8s_service.snapshot_provider.patch_snapshot_spec.return_value = {
            "metadata": {"name": sandbox_id}
        }

        # Execute
        k8s_service.resume_sandbox(sandbox_id)

        # Verify patch_snapshot_spec was called with action="Resume"
        k8s_service.snapshot_provider.patch_snapshot_spec.assert_called_once()
        call_args = k8s_service.snapshot_provider.patch_snapshot_spec.call_args
        assert call_args[1]["snapshot_name"] == sandbox_id
        assert call_args[1]["namespace"] == k8s_service.namespace
        assert call_args[1]["spec_patch"] == {"action": "Resume"}

    def test_resume_sandbox_with_resume_requested(self, k8s_service):
        """
        Test case: Resume sandbox that already has action in spec

        Purpose: Verify that resume still works correctly, setting
        action="Resume" via patch_snapshot_spec.
        """
        sandbox_id = "paused-sandbox-no-cs"

        # Mock Snapshot in Ready phase with action="Pause"
        k8s_service.snapshot_provider.get_snapshot.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "sandboxId": sandbox_id,
                "sourceBatchSandboxName": sandbox_id,
                "action": "Pause",
            },
            "status": {"phase": "Ready"},
        }

        # Mock no existing BatchSandbox
        k8s_service.workload_provider.get_workload.return_value = None

        # Mock patch success
        k8s_service.snapshot_provider.patch_snapshot_spec.return_value = {
            "metadata": {"name": sandbox_id}
        }

        # Execute
        k8s_service.resume_sandbox(sandbox_id)

        # Verify patch_snapshot_spec called with action="Resume"
        k8s_service.snapshot_provider.patch_snapshot_spec.assert_called_once()
        call_args = k8s_service.snapshot_provider.patch_snapshot_spec.call_args
        assert call_args[1]["spec_patch"] == {"action": "Resume"}

    def test_resume_sandbox_not_found(self, k8s_service):
        """
        Test case: Resume sandbox when no snapshot exists

        Purpose: Verify that 404 is returned when snapshot is not found
        """
        sandbox_id = "nonexistent-snapshot"

        # Mock no snapshot
        k8s_service.snapshot_provider.get_snapshot.return_value = None

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.resume_sandbox(sandbox_id)

        assert exc_info.value.status_code == 404
        assert exc_info.value.detail["code"] == SandboxErrorCodes.SNAPSHOT_NOT_FOUND

    def test_resume_sandbox_not_ready(self, k8s_service):
        """
        Test case: Resume sandbox when snapshot is not in Ready phase

        Purpose: Verify that 409 is returned when snapshot is not Ready
        """
        sandbox_id = "snapshot-committing"

        # Mock Snapshot in Committing phase
        k8s_service.snapshot_provider.get_snapshot.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "sandboxId": sandbox_id,
                "imageUri": "registry.example.com/snapshot-committing:snapshot",
            },
            "status": {"phase": "Committing"},
        }

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.resume_sandbox(sandbox_id)

        assert exc_info.value.status_code == 409
        assert exc_info.value.detail["code"] == SandboxErrorCodes.SNAPSHOT_NOT_READY

    def test_resume_sandbox_already_exists(self, k8s_service):
        """
        Test case: Resume sandbox when BatchSandbox already exists

        Purpose: Verify that 409 is returned when BatchSandbox already exists
        """
        sandbox_id = "existing-sandbox"

        # Mock Snapshot in Ready phase
        k8s_service.snapshot_provider.get_snapshot.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "sandboxId": sandbox_id,
                "imageUri": "registry.example.com/existing-sandbox:snapshot",
                "resumeTemplate": {
                    "template": {
                        "spec": {"containers": [{"name": "sandbox", "image": "image:latest"}]}
                    }
                },
            },
            "status": {"phase": "Ready"},
        }

        # Mock existing BatchSandbox
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {},
        }

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.resume_sandbox(sandbox_id)

        assert exc_info.value.status_code == 409
        assert exc_info.value.detail["code"] == SandboxErrorCodes.INVALID_STATE


class TestDeleteSandboxCleansSnapshot:
    """delete_sandbox cleanup tests"""

    def test_delete_sandbox_cleans_snapshot(self, k8s_service):
        """
        Test case: Delete sandbox cleans up both BatchSandbox and Snapshot

        Purpose: Verify that delete_sandbox removes both resources
        """
        sandbox_id = "sandbox-with-snapshot"

        # Mock successful deletion of both resources
        k8s_service.workload_provider.delete_workload.return_value = True
        k8s_service.snapshot_provider.delete_snapshot.return_value = True

        # Execute
        k8s_service.delete_sandbox(sandbox_id)

        # Verify both were deleted
        k8s_service.workload_provider.delete_workload.assert_called_once_with(
            sandbox_id, k8s_service.namespace
        )
        k8s_service.snapshot_provider.delete_snapshot.assert_called_once_with(
            sandbox_id, k8s_service.namespace
        )

    def test_delete_sandbox_only_workload_exists(self, k8s_service):
        """
        Test case: Delete sandbox when only BatchSandbox exists (no snapshot)

        Purpose: Verify deletion succeeds when only BatchSandbox exists
        """
        sandbox_id = "sandbox-no-snapshot"

        # Mock BatchSandbox deletion success
        k8s_service.workload_provider.delete_workload.return_value = True

        # Mock snapshot not found
        k8s_service.snapshot_provider.delete_snapshot.return_value = False

        # Execute - should not raise
        k8s_service.delete_sandbox(sandbox_id)

        # Verify both were attempted
        k8s_service.workload_provider.delete_workload.assert_called_once()
        k8s_service.snapshot_provider.delete_snapshot.assert_called_once()

    def test_delete_sandbox_only_snapshot_exists(self, k8s_service):
        """
        Test case: Delete sandbox when only Snapshot exists (no BatchSandbox)

        Purpose: Verify deletion succeeds when only Snapshot exists
        """
        sandbox_id = "paused-sandbox-no-workload"

        # Mock BatchSandbox not found
        k8s_service.workload_provider.delete_workload.return_value = False

        # Mock snapshot deletion succeeds
        k8s_service.snapshot_provider.delete_snapshot.return_value = True

        # Execute - should not raise since snapshot was deleted
        k8s_service.delete_sandbox(sandbox_id)

        # Verify both were attempted
        k8s_service.workload_provider.delete_workload.assert_called_once()
        k8s_service.snapshot_provider.delete_snapshot.assert_called_once()

    def test_delete_sandbox_not_found_raises_404(self, k8s_service):
        """
        Test case: Delete sandbox when neither resource exists

        Purpose: Verify 404 is raised when sandbox doesn't exist at all
        """
        sandbox_id = "nonexistent-sandbox"

        # Mock both resources not found
        k8s_service.workload_provider.delete_workload.return_value = False
        k8s_service.snapshot_provider.delete_snapshot.return_value = False

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.delete_sandbox(sandbox_id)

        assert exc_info.value.status_code == 404
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND


class TestDeriveSandboxState:
    """_derive_sandbox_state method tests"""

    def test_derive_state_running(self, k8s_service):
        batchsandbox = {
            "metadata": {"name": "test"},
            "status": {"replicas": 1, "ready": 1, "allocated": 1},
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "POD_READY",
            "message": "Pod is running",
        }

        state, reason, message = k8s_service._derive_sandbox_state(batchsandbox, None)

        assert state == "Running"
        assert reason == "POD_READY"

    def test_derive_state_paused(self, k8s_service):
        snapshot = {
            "metadata": {"name": "test"},
            "status": {"phase": "Ready"},
        }

        state, reason, message = k8s_service._derive_sandbox_state(None, snapshot)

        assert state == "Paused"
        assert reason == "SNAPSHOT_READY"

    def test_derive_state_pausing(self, k8s_service):
        batchsandbox = {
            "metadata": {"name": "test"},
            "status": {},
        }
        snapshot = {
            "metadata": {"name": "test"},
            "status": {"phase": "Committing"},
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "POD_READY",
            "message": "Pod is running",
        }

        state, reason, message = k8s_service._derive_sandbox_state(batchsandbox, snapshot)

        assert state == "Pausing"
        assert "COMMITTING" in reason

    def test_derive_state_resuming(self, k8s_service):
        batchsandbox = {
            "metadata": {
                "name": "test",
                "annotations": {"sandbox.opensandbox.io/resumed-from-snapshot": "true"},
            },
            "status": {},
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Pending",
            "reason": "POD_SCHEDULED",
            "message": "Pod is pending",
        }

        state, reason, message = k8s_service._derive_sandbox_state(batchsandbox, None)

        assert state == "Resuming"

    def test_derive_state_not_found(self, k8s_service):
        state, reason, message = k8s_service._derive_sandbox_state(None, None)

        assert state == "NotFound"

    def test_derive_state_snapshot_failed(self, k8s_service):
        snapshot = {
            "metadata": {"name": "test"},
            "status": {"phase": "Failed", "message": "Push failed"},
        }

        state, reason, message = k8s_service._derive_sandbox_state(None, snapshot)

        assert state == "Failed"
        assert reason == "SNAPSHOT_FAILED"

    def test_derive_state_snapshot_failed_with_running_workload(self, k8s_service):
        """P2: Snapshot failed but workload still running should return Running."""
        batchsandbox = {
            "metadata": {
                "name": "test",
                "annotations": {},
            },
            "status": {},
        }
        snapshot = {
            "metadata": {"name": "test"},
            "status": {"phase": "Failed", "message": "Push failed"},
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Running",
            "reason": "POD_RUNNING",
            "message": "Pod is running",
        }

        state, reason, message = k8s_service._derive_sandbox_state(batchsandbox, snapshot)

        # When snapshot fails but workload still exists and is running,
        # we should return the workload state, not Failed
        assert state == "Running"
        assert reason == "POD_RUNNING"

    def test_derive_state_resumed_workload_failed(self, k8s_service):
        """P1: Resumed sandbox with Failed workload should return Failed, not Resuming."""
        batchsandbox = {
            "metadata": {
                "name": "test",
                "annotations": {"sandbox.opensandbox.io/resumed-from-snapshot": "true"},
            },
            "status": {},
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Failed",
            "reason": "POD_PLATFORM_UNSCHEDULABLE",
            "message": "Pod is unschedulable",
        }

        state, reason, message = k8s_service._derive_sandbox_state(batchsandbox, None)

        assert state == "Failed"
        assert reason == "POD_PLATFORM_UNSCHEDULABLE"

    def test_derive_state_resumed_workload_pending(self, k8s_service):
        """Resumed sandbox with Pending workload should return Resuming."""
        batchsandbox = {
            "metadata": {
                "name": "test",
                "annotations": {"sandbox.opensandbox.io/resumed-from-snapshot": "true"},
            },
            "status": {},
        }
        k8s_service.workload_provider.get_status.return_value = {
            "state": "Pending",
            "reason": "BATCHSANDBOX_PENDING",
            "message": "BatchSandbox is pending allocation",
        }

        state, reason, message = k8s_service._derive_sandbox_state(batchsandbox, None)

        assert state == "Resuming"
        assert reason == "RESUMING"

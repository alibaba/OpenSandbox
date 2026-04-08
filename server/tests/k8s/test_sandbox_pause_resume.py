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

        Purpose: Verify that minimal SandboxSnapshot CR is created without
        Pod lookup, imageUri, or resumeTemplate.
        """
        sandbox_id = "test-sandbox-123"

        # Mock BatchSandbox in Running state with pausePolicy
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {
                "name": sandbox_id,
                "namespace": "test-namespace",
            },
            "spec": {
                "replicas": 1,
                "pausePolicy": {
                    "snapshotRegistry": "registry.example.com",
                    "snapshotType": "Rootfs",
                },
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
        assert "pausedAt" in snapshot_cr["spec"]

        for key in (
            "snapshotType",
            "snapshotRegistry",
            "imageUri",
            "resumeTemplate",
            "sourcePodName",
            "sourceContainerName",
            "sourceNodeName",
            "snapshotPushSecret",
            "resumeImagePullSecret",
        ):
            assert key not in snapshot_cr["spec"]

    def test_pause_sandbox_with_secrets(self, k8s_service):
        """
        Test case: Pause sandbox with push/pull secrets in pausePolicy

        Purpose: Verify secrets are NOT included in minimal CR — controller handles this
        """
        sandbox_id = "test-sandbox-secrets"

        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {
                "name": sandbox_id,
                "namespace": "test-namespace",
            },
            "spec": {
                "replicas": 1,
                "pausePolicy": {
                    "snapshotRegistry": "registry.example.com",
                    "snapshotType": "Rootfs",
                    "snapshotPushSecret": "push-secret",
                    "resumeImagePullSecret": "pull-secret",
                },
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
        assert "pausedAt" in snapshot_cr["spec"]
        assert "snapshotPushSecret" not in snapshot_cr["spec"]
        assert "resumeImagePullSecret" not in snapshot_cr["spec"]

    def test_pause_sandbox_not_found(self, k8s_service):
        """
        Test case: Pause sandbox that does not exist

        Purpose: Verify that 404 is returned when sandbox is not found
        """
        sandbox_id = "nonexistent-sandbox"

        # Mock no BatchSandbox
        k8s_service.workload_provider.get_workload.return_value = None

        # Execute and verify
        with pytest.raises(HTTPException) as exc_info:
            k8s_service.pause_sandbox(sandbox_id)

        assert exc_info.value.status_code == 404
        assert exc_info.value.detail["code"] == SandboxErrorCodes.K8S_SANDBOX_NOT_FOUND

    def test_pause_sandbox_invalid_state(self, k8s_service):
        """
        Test case: Pause sandbox that is not in Running state

        Purpose: Verify that 409 is returned when sandbox is not in Running state
        """
        sandbox_id = "pending-sandbox"

        # Mock BatchSandbox in Pending state
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "replicas": 1,
                "pausePolicy": {"snapshotRegistry": "registry.example.com"},
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
        Test case: Pause sandbox without pausePolicy configured

        Purpose: Verify that 400 is returned when sandbox has no pausePolicy
        """
        sandbox_id = "sandbox-without-pause-policy"

        # Mock BatchSandbox in Running state without pausePolicy
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "replicas": 1,
                # No pausePolicy
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

    def test_pause_sandbox_snapshot_in_progress(self, k8s_service):
        """
        Test case: Pause sandbox when snapshot is already in progress

        Purpose: Verify that 409 is returned when a snapshot is already being created
        """
        sandbox_id = "sandbox-with-snapshot-in-progress"

        # Mock BatchSandbox in Running state with pausePolicy
        k8s_service.workload_provider.get_workload.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "replicas": 1,
                "pausePolicy": {"snapshotRegistry": "registry.example.com"},
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
                "pausePolicy": {"snapshotRegistry": "registry.example.com"},
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

    def test_resume_sandbox_multi_container(self, k8s_service):
        """
        Test case: Resume sandbox with multi-container snapshots

        Purpose: Verify that BatchSandbox is created with correct images
        for each container from containerSnapshots in snapshot status.
        """
        sandbox_id = "paused-sandbox-multi"

        # Mock Snapshot in Ready phase with multi-container snapshots
        k8s_service.snapshot_provider.get_snapshot.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "sandboxId": sandbox_id,
                "resumeTemplate": {
                    "template": {
                        "spec": {
                            "containers": [
                                {"name": "sandbox", "image": "old-image:latest"},
                                {"name": "sidecar", "image": "old-sidecar:latest"},
                            ]
                        }
                    },
                    "expireTime": "2025-12-24T12:00:00Z",
                    "pausePolicy": {"snapshotRegistry": "registry.example.com"},
                },
            },
            "status": {
                "phase": "Ready",
                "containerSnapshots": [
                    {
                        "containerName": "sandbox",
                        "imageURI": "registry.example.com/paused-sandbox-multi:sandbox-snapshot",
                    },
                    {
                        "containerName": "sidecar",
                        "imageURI": "registry.example.com/paused-sandbox-multi:sidecar-snapshot",
                    },
                ],
            },
        }

        # Mock no existing BatchSandbox
        k8s_service.workload_provider.get_workload.return_value = None

        # Mock BatchSandbox creation
        k8s_service.k8s_client.create_custom_object.return_value = {
            "metadata": {"name": sandbox_id, "uid": "new-uid"}
        }

        # Set provider attributes
        k8s_service.workload_provider.group = "sandbox.opensandbox.io"
        k8s_service.workload_provider.version = "v1alpha1"
        k8s_service.workload_provider.plural = "batchsandboxes"

        # Execute
        k8s_service.resume_sandbox(sandbox_id)

        # Verify BatchSandbox was created with correct images per container
        k8s_service.k8s_client.create_custom_object.assert_called_once()
        call_args = k8s_service.k8s_client.create_custom_object.call_args
        body = call_args[1]["body"]

        assert body["metadata"]["name"] == sandbox_id
        containers = body["spec"]["template"]["spec"]["containers"]
        sandbox_container = next(c for c in containers if c["name"] == "sandbox")
        sidecar_container = next(c for c in containers if c["name"] == "sidecar")
        assert sandbox_container["image"] == (
            "registry.example.com/paused-sandbox-multi:sandbox-snapshot"
        )
        assert sidecar_container["image"] == (
            "registry.example.com/paused-sandbox-multi:sidecar-snapshot"
        )
        # Verify resumed-from-snapshot annotation
        assert (
            body["metadata"]["annotations"]["sandbox.opensandbox.io/resumed-from-snapshot"]
            == "true"
        )

    def test_resume_sandbox_no_container_snapshots(self, k8s_service):
        """
        Test case: Resume sandbox when containerSnapshots is empty

        Purpose: Verify that when containerSnapshots is empty/missing,
        original template images are preserved (no image replacement occurs).
        The controller now requires containerSnapshots — there is no legacy
        single-container fallback.
        """
        sandbox_id = "paused-sandbox-no-cs"

        # Mock Snapshot in Ready phase without containerSnapshots
        k8s_service.snapshot_provider.get_snapshot.return_value = {
            "metadata": {"name": sandbox_id},
            "spec": {
                "sandboxId": sandbox_id,
                "resumeTemplate": {
                    "template": {
                        "spec": {
                            "containers": [{"name": "sandbox", "image": "original-image:latest"}]
                        }
                    },
                    "expireTime": "2025-12-24T12:00:00Z",
                    "pausePolicy": {"snapshotRegistry": "registry.example.com"},
                },
            },
            "status": {"phase": "Ready"},
        }

        # Mock no existing BatchSandbox
        k8s_service.workload_provider.get_workload.return_value = None

        # Mock BatchSandbox creation
        k8s_service.k8s_client.create_custom_object.return_value = {
            "metadata": {"name": sandbox_id, "uid": "new-uid"}
        }

        # Set provider attributes
        k8s_service.workload_provider.group = "sandbox.opensandbox.io"
        k8s_service.workload_provider.version = "v1alpha1"
        k8s_service.workload_provider.plural = "batchsandboxes"

        # Execute
        k8s_service.resume_sandbox(sandbox_id)

        # Verify BatchSandbox was created with original images preserved
        k8s_service.k8s_client.create_custom_object.assert_called_once()
        call_args = k8s_service.k8s_client.create_custom_object.call_args
        body = call_args[1]["body"]

        assert body["metadata"]["name"] == sandbox_id
        containers = body["spec"]["template"]["spec"]["containers"]
        # Original image preserved — no replacement since containerSnapshots is empty
        assert containers[0]["image"] == "original-image:latest"

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
        k8s_service.workload_provider.delete_workload.return_value = None
        k8s_service.snapshot_provider.delete_snapshot.return_value = None

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
        k8s_service.workload_provider.delete_workload.return_value = None

        # Mock snapshot deletion raises 404 (already deleted/doesn't exist)
        k8s_service.snapshot_provider.delete_snapshot.return_value = None

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

        # Mock BatchSandbox deletion fails
        k8s_service.workload_provider.delete_workload.side_effect = Exception("not found")

        # Mock snapshot deletion succeeds
        k8s_service.snapshot_provider.delete_snapshot.return_value = None

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

        # Mock both deletions fail
        k8s_service.workload_provider.delete_workload.side_effect = Exception("not found")
        k8s_service.snapshot_provider.delete_snapshot.side_effect = Exception("not found")

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

/*
 * Copyright 2025 Alibaba Group Holding Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package com.alibaba.opensandbox.e2e;

import static org.junit.jupiter.api.Assertions.*;

import com.alibaba.opensandbox.sandbox.Sandbox;
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.*;
import com.alibaba.opensandbox.sandbox.domain.models.execd.filesystem.*;
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.*;
import java.io.ByteArrayInputStream;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.OffsetDateTime;
import java.util.*;
import java.util.concurrent.*;
import org.junit.jupiter.api.*;

/**
 * Comprehensive E2E tests for Sandbox functionality.
 *
 * <p>Tests all sandbox capabilities including - Lifecycle management (creation, health,
 * termination) - Command execution with various shells and scenarios - Filesystem operations (CRUD,
 * permissions, search) - Resource management and monitoring - Error handling and recovery -
 * Concurrent operations and stress testing
 */
@Tag("e2e")
@DisplayName("Sandbox E2E Tests (Java SDK) - Strict Coverage")
@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
public class SandboxE2ETest extends BaseE2ETest {

    private Sandbox sandbox;

    @BeforeAll
    void setup() {
        Map<String, String> resourceMap = new HashMap<>();
        resourceMap.put("cpu", "2");
        resourceMap.put("memory", "4Gi");

        Map<String, String> metadataMap = new HashMap<>();
        metadataMap.put("tag", "e2e-test");

        sandbox =
                Sandbox.builder()
                        .connectionConfig(sharedConnectionConfig)
                        .image(getSandboxImage())
                        .resource(resourceMap)
                        .timeout(Duration.ofMinutes(2))
                        .readyTimeout(Duration.ofSeconds(60))
                        .metadata(metadataMap)
                        .env("E2E_TEST", "true")
                        .healthCheckPollingInterval(Duration.ofMillis(500))
                        .build();
    }

    @AfterAll
    void teardown() {
        if (sandbox != null) {
            try {
                sandbox.kill();
            } catch (Exception ignored) {
            }
            try {
                sandbox.close();
            } catch (Exception ignored) {
            }
        }
    }

    private static void assertModifiedUpdated(
            OffsetDateTime before, OffsetDateTime after, long minDeltaMs, long allowSkewMs) {
        long deltaMs = Duration.between(before, after).toMillis();
        assertTrue(
                deltaMs >= minDeltaMs - allowSkewMs,
                "modifiedAt did not update as expected: deltaMs="
                        + deltaMs
                        + " (minDeltaMs="
                        + minDeltaMs
                        + ", allowSkewMs="
                        + allowSkewMs
                        + ")");
    }

    private static void assertTerminalEventContract(
            List<ExecutionInit> initEvents,
            List<ExecutionComplete> completedEvents,
            List<ExecutionError> errors,
            String executionId) {
        assertEquals(1, initEvents.size(), "Execution must have exactly one init event");
        assertNotNull(initEvents.get(0).getId());
        assertFalse(initEvents.get(0).getId().isBlank());
        assertEquals(executionId, initEvents.get(0).getId(), "init.id must match execution.id");
        assertRecentTimestampMs(initEvents.get(0).getTimestamp(), 120_000);

        boolean hasComplete = !completedEvents.isEmpty();
        boolean hasError = !errors.isEmpty();
        assertTrue(
                hasComplete || hasError,
                "expected at least one of complete/error, got complete="
                        + completedEvents.size()
                        + " error="
                        + errors.size());
        if (hasComplete) {
            assertEquals(1, completedEvents.size());
            assertRecentTimestampMs(completedEvents.get(0).getTimestamp(), 180_000);
            assertTrue(completedEvents.get(0).getExecutionTimeInMillis() >= 0);
        }
        if (hasError) {
            assertNotNull(errors.get(0).getName());
            assertFalse(errors.get(0).getName().isBlank());
            assertNotNull(errors.get(0).getValue());
            assertRecentTimestampMs(errors.get(0).getTimestamp(), 180_000);
        }
    }

    @Test
    @Order(1)
    @DisplayName("Sandbox lifecycle, health, endpoint, metrics, renew, connect")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testSandboxLifecycleAndHealth() {
        assertNotNull(sandbox);
        assertNotNull(sandbox.getId());
        assertTrue(sandbox.isHealthy(), "Sandbox should be healthy");

        SandboxInfo info = sandbox.getInfo();
        assertEquals(sandbox.getId(), info.getId());
        assertEquals("Running", info.getStatus().getState());
        assertNotNull(info.getCreatedAt());
        assertNotNull(info.getExpiresAt());
        assertTrue(info.getExpiresAt().isAfter(info.getCreatedAt()));
        assertEquals(List.of("tail", "-f", "/dev/null"), info.getEntrypoint());

        Duration duration = Duration.between(info.getCreatedAt(), info.getExpiresAt());
        assertTrue(duration.compareTo(Duration.ofMinutes(1)) >= 0);
        assertTrue(duration.compareTo(Duration.ofMinutes(3)) <= 0);

        assertNotNull(info.getMetadata());
        assertEquals("e2e-test", info.getMetadata().get("tag"));

        SandboxEndpoint endpoint = sandbox.getEndpoint(44772);
        assertNotNull(endpoint);
        assertEndpointHasPort(endpoint.getEndpoint(), 44772);

        SandboxMetrics metrics = sandbox.getMetrics();
        assertNotNull(metrics);
        assertTrue(metrics.getCpuCount() > 0);
        assertTrue(
                metrics.getCpuUsedPercentage() >= 0.0 && metrics.getCpuUsedPercentage() <= 100.0);
        assertTrue(metrics.getMemoryTotalInMiB() > 0);
        assertTrue(
                metrics.getMemoryUsedInMiB() >= 0.0
                        && metrics.getMemoryUsedInMiB() <= metrics.getMemoryTotalInMiB());
        assertRecentTimestampMs(metrics.getTimestamp(), 120_000);

        // Renew: validate remaining TTL is close to requested duration.
        SandboxRenewResponse renewResp = sandbox.renew(Duration.ofMinutes(5));
        assertNotNull(renewResp, "renew() should return a response");
        assertNotNull(renewResp.getExpiresAt(), "renew().expiresAt should not be null");
        SandboxInfo renewedInfo = sandbox.getInfo();
        assertTrue(renewedInfo.getExpiresAt().isAfter(info.getExpiresAt()));
        assertTrue(
                renewResp.getExpiresAt().isAfter(info.getExpiresAt()),
                "renew().expiresAt should be after previous expiresAt");
        // Allow small skew between renew response and subsequent getInfo() (backend timing).
        assertTrue(
                Math.abs(
                                Duration.between(
                                                renewResp.getExpiresAt(),
                                                renewedInfo.getExpiresAt())
                                        .toSeconds())
                        < 10,
                "renew response expiresAt should be close to getInfo().expiresAt");
        Duration remaining = Duration.between(OffsetDateTime.now(), renewedInfo.getExpiresAt());
        assertTrue(
                remaining.compareTo(Duration.ofMinutes(3)) > 0,
                "Remaining TTL too small: " + remaining);
        assertTrue(
                remaining.compareTo(Duration.ofMinutes(6)) < 0,
                "Remaining TTL too large: " + remaining);

        assertNotNull(sandbox.files());
        assertNotNull(sandbox.commands());
        assertNotNull(sandbox.metrics());
        assertNotNull(sandbox.httpClientProvider());

        // Connect to existing sandbox by ID and run a basic command.
        Sandbox sandbox2 =
                Sandbox.connector()
                        .connectionConfig(sharedConnectionConfig)
                        .sandboxId(sandbox.getId())
                        .connect();
        try {
            assertEquals(sandbox.getId(), sandbox2.getId());
            assertTrue(sandbox2.isHealthy());
            Execution r =
                    sandbox2.commands()
                            .run(RunCommandRequest.builder().command("echo connect-ok").build());
            assertNotNull(r);
            assertNull(r.getError());
            assertEquals(1, r.getLogs().getStdout().size());
            assertEquals("connect-ok", r.getLogs().getStdout().get(0).getText());
        } finally {
            sandbox2.close();
        }
    }

    @Test
    @Order(2)
    @DisplayName("Sandbox create with networkPolicy")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testSandboxCreateWithNetworkPolicy() {
        NetworkPolicy networkPolicy =
                NetworkPolicy.builder()
                        .defaultAction(NetworkPolicy.DefaultAction.DENY)
                        .addEgress(
                                NetworkRule.builder()
                                        .action(NetworkRule.Action.ALLOW)
                                        .target("pypi.org")
                                        .build())
                        .build();

        Sandbox policySandbox =
                Sandbox.builder()
                        .connectionConfig(sharedConnectionConfig)
                        .image(getSandboxImage())
                        .timeout(Duration.ofMinutes(2))
                        .readyTimeout(Duration.ofSeconds(60))
                        .networkPolicy(networkPolicy)
                        .build();
        // Wait for NetworkPolicy sidecar to be fully initialized
        try {
            Thread.sleep(2000);
        } catch (InterruptedException ignored) {
        }

        try {
            Execution r =
                    policySandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command("curl -I https://www.github.com")
                                            .build());
            assertNotNull(r);
            assertNotNull(r.getError());

            r =
                    policySandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command("curl -I https://pypi.org")
                                            .build());
            assertNotNull(r);
            assertNull(r.getError());
        } finally {
            try {
                policySandbox.kill();
            } catch (Exception ignored) {
            }
            policySandbox.close();
        }
    }

    @Test
    @Order(2)
    @DisplayName("Sandbox create with host volume mount (read-write)")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testSandboxCreateWithHostVolumeMount() {
        String hostDir = "/tmp/opensandbox-e2e/host-volume-test";
        String containerMountPath = "/mnt/host-data";

        Volume volume =
                Volume.builder()
                        .name("test-host-vol")
                        .host(Host.of(hostDir))
                        .mountPath(containerMountPath)
                        .readOnly(false)
                        .build();

        Sandbox volumeSandbox =
                Sandbox.builder()
                        .connectionConfig(sharedConnectionConfig)
                        .image(getSandboxImage())
                        .timeout(Duration.ofMinutes(2))
                        .readyTimeout(Duration.ofSeconds(60))
                        .volume(volume)
                        .build();

        try {
            assertTrue(volumeSandbox.isHealthy(), "Volume sandbox should be healthy");

            // Step 1: Verify the host marker file is visible inside the sandbox
            Execution readMarker =
                    volumeSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command("cat " + containerMountPath + "/marker.txt")
                                            .build());
            assertNull(readMarker.getError(), "Failed to read marker file");
            assertEquals(1, readMarker.getLogs().getStdout().size());
            assertEquals(
                    "opensandbox-e2e-marker",
                    readMarker.getLogs().getStdout().get(0).getText());

            // Step 2: Write a file from inside the sandbox to the mounted path
            Execution writeResult =
                    volumeSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command(
                                                    "echo 'written-from-sandbox' > "
                                                            + containerMountPath
                                                            + "/sandbox-output.txt")
                                            .build());
            assertNull(writeResult.getError(), "Failed to write file");

            // Step 3: Verify the written file is readable
            Execution readBack =
                    volumeSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command(
                                                    "cat "
                                                            + containerMountPath
                                                            + "/sandbox-output.txt")
                                            .build());
            assertNull(readBack.getError());
            assertEquals(1, readBack.getLogs().getStdout().size());
            assertEquals(
                    "written-from-sandbox", readBack.getLogs().getStdout().get(0).getText());

            // Step 4: Verify the mount path is a proper directory
            Execution dirCheck =
                    volumeSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command(
                                                    "test -d " + containerMountPath + " && echo OK")
                                            .build());
            assertNull(dirCheck.getError());
            assertEquals(1, dirCheck.getLogs().getStdout().size());
            assertEquals("OK", dirCheck.getLogs().getStdout().get(0).getText());
        } finally {
            try {
                volumeSandbox.kill();
            } catch (Exception ignored) {
            }
            volumeSandbox.close();
        }
    }

    @Test
    @Order(2)
    @DisplayName("Sandbox create with host volume mount (read-only)")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testSandboxCreateWithHostVolumeMountReadOnly() {
        String hostDir = "/tmp/opensandbox-e2e/host-volume-test";
        String containerMountPath = "/mnt/host-data-ro";

        Volume volume =
                Volume.builder()
                        .name("test-host-vol-ro")
                        .host(Host.of(hostDir))
                        .mountPath(containerMountPath)
                        .readOnly(true)
                        .build();

        Sandbox roSandbox =
                Sandbox.builder()
                        .connectionConfig(sharedConnectionConfig)
                        .image(getSandboxImage())
                        .timeout(Duration.ofMinutes(2))
                        .readyTimeout(Duration.ofSeconds(60))
                        .volume(volume)
                        .build();

        try {
            assertTrue(roSandbox.isHealthy(), "Read-only volume sandbox should be healthy");

            // Step 1: Verify the host marker file is readable
            Execution readMarker =
                    roSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command("cat " + containerMountPath + "/marker.txt")
                                            .build());
            assertNull(readMarker.getError(), "Failed to read marker file on read-only mount");
            assertEquals(1, readMarker.getLogs().getStdout().size());
            assertEquals(
                    "opensandbox-e2e-marker",
                    readMarker.getLogs().getStdout().get(0).getText());

            // Step 2: Verify writing is denied on read-only mount
            Execution writeResult =
                    roSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command(
                                                    "touch "
                                                            + containerMountPath
                                                            + "/should-fail.txt")
                                            .build());
            assertNotNull(
                    writeResult.getError(), "Write should fail on read-only mount");
        } finally {
            try {
                roSandbox.kill();
            } catch (Exception ignored) {
            }
            roSandbox.close();
        }
    }

    @Test
    @Order(2)
    @DisplayName("Sandbox create with PVC named volume mount (read-write)")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testSandboxCreateWithPvcVolumeMount() {
        String pvcVolumeName = "opensandbox-e2e-pvc-test";
        String containerMountPath = "/mnt/pvc-data";

        Volume volume =
                Volume.builder()
                        .name("test-pvc-vol")
                        .pvc(PVC.of(pvcVolumeName))
                        .mountPath(containerMountPath)
                        .readOnly(false)
                        .build();

        Sandbox pvcSandbox =
                Sandbox.builder()
                        .connectionConfig(sharedConnectionConfig)
                        .image(getSandboxImage())
                        .timeout(Duration.ofMinutes(2))
                        .readyTimeout(Duration.ofSeconds(60))
                        .volume(volume)
                        .build();

        try {
            assertTrue(pvcSandbox.isHealthy(), "PVC volume sandbox should be healthy");

            // Step 1: Verify the marker file seeded into the named volume is readable
            Execution readMarker =
                    pvcSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command("cat " + containerMountPath + "/marker.txt")
                                            .build());
            assertNull(readMarker.getError(), "Failed to read marker file from PVC volume");
            assertEquals(1, readMarker.getLogs().getStdout().size());
            assertEquals(
                    "pvc-marker-data",
                    readMarker.getLogs().getStdout().get(0).getText());

            // Step 2: Write a file from inside the sandbox to the named volume
            Execution writeResult =
                    pvcSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command(
                                                    "echo 'written-to-pvc' > "
                                                            + containerMountPath
                                                            + "/pvc-output.txt")
                                            .build());
            assertNull(writeResult.getError(), "Failed to write file to PVC volume");

            // Step 3: Verify the written file is readable
            Execution readBack =
                    pvcSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command(
                                                    "cat "
                                                            + containerMountPath
                                                            + "/pvc-output.txt")
                                            .build());
            assertNull(readBack.getError());
            assertEquals(1, readBack.getLogs().getStdout().size());
            assertEquals(
                    "written-to-pvc", readBack.getLogs().getStdout().get(0).getText());

            // Step 4: Verify the mount path is a proper directory
            Execution dirCheck =
                    pvcSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command(
                                                    "test -d " + containerMountPath + " && echo OK")
                                            .build());
            assertNull(dirCheck.getError());
            assertEquals(1, dirCheck.getLogs().getStdout().size());
            assertEquals("OK", dirCheck.getLogs().getStdout().get(0).getText());
        } finally {
            try {
                pvcSandbox.kill();
            } catch (Exception ignored) {
            }
            pvcSandbox.close();
        }
    }

    @Test
    @Order(2)
    @DisplayName("Sandbox create with PVC named volume mount (read-only)")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testSandboxCreateWithPvcVolumeMountReadOnly() {
        String pvcVolumeName = "opensandbox-e2e-pvc-test";
        String containerMountPath = "/mnt/pvc-data-ro";

        Volume volume =
                Volume.builder()
                        .name("test-pvc-vol-ro")
                        .pvc(PVC.of(pvcVolumeName))
                        .mountPath(containerMountPath)
                        .readOnly(true)
                        .build();

        Sandbox roSandbox =
                Sandbox.builder()
                        .connectionConfig(sharedConnectionConfig)
                        .image(getSandboxImage())
                        .timeout(Duration.ofMinutes(2))
                        .readyTimeout(Duration.ofSeconds(60))
                        .volume(volume)
                        .build();

        try {
            assertTrue(roSandbox.isHealthy(), "Read-only PVC volume sandbox should be healthy");

            // Step 1: Verify the marker file is readable
            Execution readMarker =
                    roSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command("cat " + containerMountPath + "/marker.txt")
                                            .build());
            assertNull(readMarker.getError(), "Failed to read marker file on read-only PVC mount");
            assertEquals(1, readMarker.getLogs().getStdout().size());
            assertEquals(
                    "pvc-marker-data",
                    readMarker.getLogs().getStdout().get(0).getText());

            // Step 2: Verify writing is denied on read-only mount
            Execution writeResult =
                    roSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command(
                                                    "touch "
                                                            + containerMountPath
                                                            + "/should-fail.txt")
                                            .build());
            assertNotNull(
                    writeResult.getError(), "Write should fail on read-only PVC mount");
        } finally {
            try {
                roSandbox.kill();
            } catch (Exception ignored) {
            }
            roSandbox.close();
        }
    }

    @Test
    @Order(2)
    @DisplayName("Sandbox create with PVC named volume subPath mount")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testSandboxCreateWithPvcVolumeMountSubPath() {
        String pvcVolumeName = "opensandbox-e2e-pvc-test";
        String containerMountPath = "/mnt/train";

        Volume volume =
                Volume.builder()
                        .name("test-pvc-subpath")
                        .pvc(PVC.of(pvcVolumeName))
                        .mountPath(containerMountPath)
                        .readOnly(false)
                        .subPath("datasets/train")
                        .build();

        Sandbox subpathSandbox =
                Sandbox.builder()
                        .connectionConfig(sharedConnectionConfig)
                        .image(getSandboxImage())
                        .timeout(Duration.ofMinutes(2))
                        .readyTimeout(Duration.ofSeconds(60))
                        .volume(volume)
                        .build();

        try {
            assertTrue(subpathSandbox.isHealthy(), "PVC subPath sandbox should be healthy");

            // Step 1: Verify the subpath marker file is readable
            Execution readMarker =
                    subpathSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command("cat " + containerMountPath + "/marker.txt")
                                            .build());
            assertNull(readMarker.getError(), "Failed to read subpath marker file");
            assertEquals(1, readMarker.getLogs().getStdout().size());
            assertEquals(
                    "pvc-subpath-marker",
                    readMarker.getLogs().getStdout().get(0).getText());

            // Step 2: Verify only subPath contents are visible (not the full volume)
            Execution lsResult =
                    subpathSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command("ls " + containerMountPath + "/")
                                            .build());
            assertNull(lsResult.getError());
            String lsOutput =
                    lsResult.getLogs().getStdout().stream()
                            .map(m -> m.getText())
                            .reduce("", (a, b) -> a + "\n" + b);
            assertTrue(lsOutput.contains("marker.txt"), "Should contain marker.txt");
            assertFalse(lsOutput.contains("datasets"), "Should not contain datasets dir");

            // Step 3: Write a file and verify
            Execution writeResult =
                    subpathSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command(
                                                    "echo 'subpath-write-test' > "
                                                            + containerMountPath
                                                            + "/output.txt")
                                            .build());
            assertNull(writeResult.getError(), "Failed to write file to PVC subPath");

            Execution readBack =
                    subpathSandbox
                            .commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command(
                                                    "cat "
                                                            + containerMountPath
                                                            + "/output.txt")
                                            .build());
            assertNull(readBack.getError());
            assertEquals(1, readBack.getLogs().getStdout().size());
            assertEquals(
                    "subpath-write-test", readBack.getLogs().getStdout().get(0).getText());
        } finally {
            try {
                subpathSandbox.kill();
            } catch (Exception ignored) {
            }
            subpathSandbox.close();
        }
    }

    // ==========================================
    // Command Execution Tests
    // ==========================================

    @Test
    @Order(3)
    @DisplayName("Command execution: success, cwd, background, failure")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testBasicCommandExecution() {
        assertNotNull(sandbox);

        List<OutputMessage> stdoutMessages = Collections.synchronizedList(new ArrayList<>());
        List<OutputMessage> stderrMessages = Collections.synchronizedList(new ArrayList<>());
        List<ExecutionResult> results = Collections.synchronizedList(new ArrayList<>());
        List<ExecutionError> errors = Collections.synchronizedList(new ArrayList<>());
        List<ExecutionComplete> completedEvents = Collections.synchronizedList(new ArrayList<>());
        List<ExecutionInit> initEvents = Collections.synchronizedList(new ArrayList<>());

        ExecutionHandlers handlers =
                ExecutionHandlers.builder()
                        .onStdout(
                                (OutputMessage msg) -> {
                                    stdoutMessages.add(msg);
                                    logger.info("Stdout: {}", msg.getText());
                                })
                        .onStderr(
                                (OutputMessage msg) -> {
                                    stderrMessages.add(msg);
                                    logger.warn("Stderr: {}", msg.getText());
                                })
                        .onResult(
                                (ExecutionResult result) -> {
                                    results.add(result);
                                })
                        .onExecutionComplete(
                                (ExecutionComplete complete) -> {
                                    completedEvents.add(complete);
                                })
                        .onError(
                                (ExecutionError error) -> {
                                    errors.add(error);
                                })
                        .onInit(
                                (ExecutionInit init) -> {
                                    initEvents.add(init);
                                })
                        .build();

        RunCommandRequest echoRequest =
                RunCommandRequest.builder()
                        .command("echo 'Hello OpenSandbox E2E'")
                        .handlers(handlers)
                        .build();
        Execution echoResult = sandbox.commands().run(echoRequest);

        assertNotNull(echoResult);
        assertNotNull(echoResult.getId());
        assertFalse(echoResult.getId().isBlank());
        assertNull(echoResult.getError());
        assertEquals(1, echoResult.getLogs().getStdout().size());
        assertEquals("Hello OpenSandbox E2E", echoResult.getLogs().getStdout().get(0).getText());
        assertFalse(echoResult.getLogs().getStdout().get(0).isError());
        assertRecentTimestampMs(echoResult.getLogs().getStdout().get(0).getTimestamp(), 60_000);
        assertEquals(0, echoResult.getLogs().getStderr().size());

        assertTerminalEventContract(initEvents, completedEvents, errors, echoResult.getId());
        assertEquals(1, stdoutMessages.size());
        assertEquals("Hello OpenSandbox E2E", stdoutMessages.get(0).getText());
        assertFalse(stdoutMessages.get(0).isError());
        assertRecentTimestampMs(stdoutMessages.get(0).getTimestamp(), 60_000);
        assertTrue(stderrMessages.isEmpty());

        RunCommandRequest pwdRequest =
                RunCommandRequest.builder().command("pwd").workingDirectory("/tmp").build();

        Execution pwdResult = sandbox.commands().run(pwdRequest);
        assertNotNull(pwdResult);
        assertNotNull(pwdResult.getId());
        assertNull(pwdResult.getError());
        assertEquals(1, pwdResult.getLogs().getStdout().size());
        assertEquals("/tmp", pwdResult.getLogs().getStdout().get(0).getText());
        assertFalse(pwdResult.getLogs().getStdout().get(0).isError());
        assertRecentTimestampMs(pwdResult.getLogs().getStdout().get(0).getTimestamp(), 60_000);

        long startTime = System.currentTimeMillis();
        RunCommandRequest backgroundRequest =
                RunCommandRequest.builder().command("sleep 30").background(true).build();

        sandbox.commands().run(backgroundRequest);
        long endTime = System.currentTimeMillis();

        long executionTime = endTime - startTime;
        assertTrue(
                executionTime < 10000,
                String.format(
                        "Background command should return quickly, but took %d ms", executionTime));

        // Failure case: contract error OR complete (mutually exclusive) and error must be present.
        stdoutMessages.clear();
        stderrMessages.clear();
        results.clear();
        errors.clear();
        completedEvents.clear();
        initEvents.clear();
        RunCommandRequest failRequest =
                RunCommandRequest.builder()
                        .command("nonexistent-command-that-does-not-exist")
                        .handlers(handlers)
                        .build();
        Execution failResult = sandbox.commands().run(failRequest);
        assertNotNull(failResult);
        assertNotNull(failResult.getId());
        assertFalse(failResult.getId().isBlank());
        assertNotNull(failResult.getError());
        assertEquals("CommandExecError", failResult.getError().getName());
        assertTrue(failResult.getLogs().getStderr().size() > 0);
        assertTrue(
                failResult.getLogs().getStderr().stream()
                        .anyMatch(
                                m ->
                                        m.getText()
                                                .contains(
                                                        "nonexistent-command-that-does-not-exist")));
        assertTrue(failResult.getLogs().getStderr().stream().allMatch(OutputMessage::isError));
        assertRecentTimestampMs(failResult.getLogs().getStderr().get(0).getTimestamp(), 60_000);

        assertTerminalEventContract(initEvents, completedEvents, errors, failResult.getId());
        assertTrue(completedEvents.isEmpty(), "Failing command should not emit completion event");
    }

    // ==========================================
    // Filesystem Operations Tests
    // ==========================================

    @Test
    @Order(4)
    @DisplayName("Command status + background logs")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testCommandStatusAndLogs() throws Exception {
        assertNotNull(sandbox);

        RunCommandRequest backgroundRequest =
                RunCommandRequest.builder()
                        .command("sh -c 'echo log-line-1; echo log-line-2; sleep 2'")
                        .background(true)
                        .build();
        Execution exec = sandbox.commands().run(backgroundRequest);
        assertNotNull(exec.getId());
        String commandId = exec.getId();

        CommandStatus status = sandbox.commands().getCommandStatus(commandId);
        String statusId = status.getId();
        Boolean runningValue = status.getRunning();
        assertEquals(commandId, statusId);
        assertNotNull(runningValue);

        StringBuilder logsText = new StringBuilder();
        Long cursor = null;
        for (int i = 0; i < 20; i++) {
            CommandLogs logs = sandbox.commands().getBackgroundCommandLogs(commandId, cursor);
            String content = logs.getContent();
            cursor = logs.getCursor();
            logsText.append(content);
            if (logsText.toString().contains("log-line-2")) {
                break;
            }
            Thread.sleep(1000);
        }

        assertTrue(logsText.toString().contains("log-line-1"));
        assertTrue(logsText.toString().contains("log-line-2"));
    }

    @Test
    @Order(5)
    @DisplayName("Filesystem operations: CRUD + replace/move/delete + mtime checks")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testBasicFilesystemOperations() {
        assertNotNull(sandbox);
        String testDir1 = "/tmp/fs_test1_" + System.currentTimeMillis();
        String testDir2 = "/tmp/fs_test2_" + System.currentTimeMillis();

        WriteEntry dirEntry1 = WriteEntry.builder().path(testDir1).mode(755).build();
        WriteEntry dirEntry2 = WriteEntry.builder().path(testDir2).mode(644).build();

        sandbox.files().createDirectories(List.of(dirEntry1, dirEntry2));

        Map<String, EntryInfo> dirInfo = sandbox.files().readFileInfo(List.of(testDir1, testDir2));
        assertEquals(testDir1, dirInfo.get(testDir1).getPath());
        assertEquals(755, dirInfo.get(testDir1).getMode());
        assertTimesClose(
                dirInfo.get(testDir1).getCreatedAt(), dirInfo.get(testDir1).getModifiedAt(), 2);

        Execution lsResult =
                sandbox.commands()
                        .run(
                                RunCommandRequest.builder()
                                        .command("ls -la |grep fs_test")
                                        .workingDirectory("/tmp")
                                        .build());

        assertEquals(2, lsResult.getLogs().getStdout().size());

        String testFile1 = testDir1 + "/test_file1.txt";
        String testFile2 = testDir1 + "/test_file2.txt";
        String testFile3 = testDir1 + "/test_file3.txt";
        String testContent = "Hello Filesystem!\nLine 2 with special chars: åäö\nLine 3";

        WriteEntry writeEntry1 =
                WriteEntry.builder().path(testFile1).data(testContent).mode(644).build();
        WriteEntry writeEntry2 =
                WriteEntry.builder()
                        .path(testFile2)
                        .data(testContent.getBytes(StandardCharsets.UTF_8))
                        .mode(755)
                        .build();
        WriteEntry writeEntry3 =
                WriteEntry.builder()
                        .path(testFile3)
                        .data(
                                new ByteArrayInputStream(
                                        testContent.getBytes(StandardCharsets.UTF_8)))
                        .group("nogroup")
                        .owner("nobody")
                        .mode(755)
                        .build();

        sandbox.files().write(List.of(writeEntry1, writeEntry2, writeEntry3));

        String readContent1 =
                sandbox.files().readFile(testFile1, StandardCharsets.UTF_8.name(), null);
        String readContent1Partial =
                sandbox.files().readFile(testFile1, StandardCharsets.UTF_8.name(), "bytes=0-9");

        byte[] readBytes2 = sandbox.files().readByteArray(testFile2, null);
        String readContent2 = new String(readBytes2, StandardCharsets.UTF_8);

        try (java.io.InputStream inputStream = sandbox.files().readStream(testFile3, null)) {
            byte[] streamBytes = inputStream.readAllBytes();
            String readContent3 = new String(streamBytes, StandardCharsets.UTF_8);

            // Verify content matches original for all files
            assertEquals(testContent, readContent1, "Content of testFile1 should match");
            assertEquals(testContent, readContent2, "Content of testFile2 should match");
            assertEquals(testContent, readContent3, "Content of testFile3 should match");

            // Verify partial read works correctly
            assertEquals(
                    testContent.substring(0, 10),
                    readContent1Partial,
                    "Partial read should match first 10 characters");
        } catch (java.io.IOException e) {
            throw new RuntimeException("Failed to read stream", e);
        }

        List<String> allTestFiles = List.of(testFile1, testFile2, testFile3);
        Map<String, EntryInfo> fileInfoMap = sandbox.files().readFileInfo(allTestFiles);
        long expectedSize = testContent.getBytes(StandardCharsets.UTF_8).length;

        EntryInfo fileInfo1 = fileInfoMap.get(testFile1);
        assertNotNull(fileInfo1, "FileInfo for testFile1 should not be null");
        assertEquals(testFile1, fileInfo1.getPath());
        assertEquals(expectedSize, fileInfo1.getSize(), "File1 size should match content length");
        assertEquals(644, fileInfo1.getMode(), "File1 mode should be 644");
        assertNotNull(fileInfo1.getOwner(), "File1 owner should not be null");
        assertNotNull(fileInfo1.getGroup(), "File1 group should not be null");
        assertTimesClose(fileInfo1.getCreatedAt(), fileInfo1.getModifiedAt(), 2);

        EntryInfo fileInfo2 = fileInfoMap.get(testFile2);
        assertNotNull(fileInfo2, "FileInfo for testFile2 should not be null");
        assertEquals(testFile2, fileInfo2.getPath());
        assertEquals(expectedSize, fileInfo2.getSize(), "File2 size should match content length");
        assertEquals(755, fileInfo2.getMode(), "File2 mode should be 755");
        assertNotNull(fileInfo2.getOwner(), "File2 owner should not be null");
        assertNotNull(fileInfo2.getGroup(), "File2 group should not be null");
        assertTimesClose(fileInfo2.getCreatedAt(), fileInfo2.getModifiedAt(), 2);

        EntryInfo fileInfo3 = fileInfoMap.get(testFile3);
        assertNotNull(fileInfo3, "FileInfo for testFile3 should not be null");
        assertEquals(testFile3, fileInfo3.getPath());
        assertEquals(expectedSize, fileInfo3.getSize(), "File3 size should match content length");
        assertEquals(755, fileInfo3.getMode(), "File3 mode should be 755");
        assertEquals("nobody", fileInfo3.getOwner(), "File3 owner should be nobody");
        assertEquals("nogroup", fileInfo3.getGroup(), "File3 group should be nogroup");
        assertTimesClose(fileInfo3.getCreatedAt(), fileInfo3.getModifiedAt(), 2);

        SearchEntry searchAllEntry = SearchEntry.builder().path(testDir1).pattern("*").build();
        Set<String> found = new HashSet<>();
        for (EntryInfo e : sandbox.files().search(searchAllEntry)) {
            found.add(e.getPath());
        }
        assertEquals(Set.of(testFile1, testFile2, testFile3), found);

        SetPermissionEntry permEntry1 =
                SetPermissionEntry.builder()
                        .path(testFile1)
                        .mode(755)
                        .owner("nobody")
                        .group("nogroup")
                        .build();
        SetPermissionEntry permEntry2 =
                SetPermissionEntry.builder()
                        .path(testFile2)
                        .mode(600)
                        .owner("nobody")
                        .group("nogroup")
                        .build();
        sandbox.files().setPermissions(List.of(permEntry1, permEntry2));

        // Verify permission changes for both files in single call
        Map<String, EntryInfo> updatedInfoMap =
                sandbox.files().readFileInfo(List.of(testFile1, testFile2));
        EntryInfo updatedInfo1 = updatedInfoMap.get(testFile1);
        EntryInfo updatedInfo2 = updatedInfoMap.get(testFile2);

        assertNotNull(updatedInfo1, "Updated info for testFile1 should not be null");
        assertEquals(755, updatedInfo1.getMode(), "testFile1 mode should be updated to 755");
        assertEquals(
                "nobody", updatedInfo1.getOwner(), "testFile1 owner should be updated to nobody");
        assertEquals(
                "nogroup", updatedInfo1.getGroup(), "testFile1 group should be updated to nogroup");

        assertNotNull(updatedInfo2, "Updated info for testFile2 should not be null");
        assertEquals(600, updatedInfo2.getMode(), "testFile2 mode should be updated to 600");
        assertEquals(
                "nobody", updatedInfo2.getOwner(), "testFile2 owner should be updated to nobody");
        assertEquals(
                "nogroup", updatedInfo2.getGroup(), "testFile2 group should be updated to nogroup");

        EntryInfo beforeUpdate = sandbox.files().readFileInfo(List.of(testFile1)).get(testFile1);
        String updatedContent1 = testContent + "\nAppended line to file1";
        String updatedContent2 = testContent + "\nAppended line to file2";
        try {
            Thread.sleep(50);
        } catch (InterruptedException ignored) {
        }
        WriteEntry updateEntry1 =
                WriteEntry.builder().path(testFile1).data(updatedContent1).mode(644).build();
        WriteEntry updateEntry2 =
                WriteEntry.builder().path(testFile2).data(updatedContent2).mode(755).build();
        sandbox.files().write(List.of(updateEntry1, updateEntry2));

        String newContent1 = sandbox.files().readFile(testFile1, "UTF-8", null);
        String newContent2 = sandbox.files().readFile(testFile2, "UTF-8", null);
        assertEquals(updatedContent1, newContent1);
        assertEquals(updatedContent2, newContent2);

        EntryInfo afterUpdate = sandbox.files().readFileInfo(List.of(testFile1)).get(testFile1);
        assertEquals(
                updatedContent1.getBytes(StandardCharsets.UTF_8).length, afterUpdate.getSize());
        assertModifiedUpdated(beforeUpdate.getModifiedAt(), afterUpdate.getModifiedAt(), 1, 1000);

        // Replace contents
        EntryInfo beforeReplace = afterUpdate;
        try {
            Thread.sleep(50);
        } catch (InterruptedException ignored) {
        }
        sandbox.files()
                .replaceContents(
                        List.of(
                                ContentReplaceEntry.builder()
                                        .path(testFile1)
                                        .oldContent("Appended line to file1")
                                        .newContent("Replaced line in file1")
                                        .build()));
        String replaced = sandbox.files().readFile(testFile1, "UTF-8", null);
        assertTrue(replaced.contains("Replaced line in file1"));
        assertFalse(replaced.contains("Appended line to file1"));
        EntryInfo afterReplace = sandbox.files().readFileInfo(List.of(testFile1)).get(testFile1);
        assertModifiedUpdated(beforeReplace.getModifiedAt(), afterReplace.getModifiedAt(), 1, 1000);

        // Move file3
        String movedPath = testDir2 + "/moved_file3.txt";
        sandbox.files()
                .moveFiles(List.of(MoveEntry.builder().src(testFile3).dest(movedPath).build()));
        String moved =
                new String(sandbox.files().readByteArray(movedPath, null), StandardCharsets.UTF_8);
        assertEquals(testContent, moved);
        assertThrows(Exception.class, () -> sandbox.files().readByteArray(testFile3, null));

        // Delete file2
        sandbox.files().deleteFiles(List.of(testFile2));
        assertThrows(Exception.class, () -> sandbox.files().readFile(testFile2, "UTF-8", null));
        Set<String> after = new HashSet<>();
        for (EntryInfo e :
                sandbox.files().search(SearchEntry.builder().path(testDir1).pattern("*").build())) {
            after.add(e.getPath());
        }
        assertEquals(Set.of(testFile1), after);

        // Delete directories
        sandbox.files().deleteDirectories(List.of(testDir1, testDir2));
        Execution verify =
                sandbox.commands()
                        .run(
                                RunCommandRequest.builder()
                                        .command(
                                                "test ! -d "
                                                        + testDir1
                                                        + " && test ! -d "
                                                        + testDir2
                                                        + " && echo OK")
                                        .workingDirectory("/tmp")
                                        .build());
        assertNull(verify.getError());
        assertEquals(1, verify.getLogs().getStdout().size());
        assertEquals("OK", verify.getLogs().getStdout().get(0).getText());
    }

    @Test
    @Order(6)
    @DisplayName("Interrupt command")
    @Timeout(value = 2, unit = TimeUnit.MINUTES)
    void testInterruptCommand() throws Exception {
        assertNotNull(sandbox);

        List<ExecutionInit> initEvents = Collections.synchronizedList(new ArrayList<>());
        List<ExecutionComplete> completedEvents = Collections.synchronizedList(new ArrayList<>());
        List<ExecutionError> errors = Collections.synchronizedList(new ArrayList<>());
        CountDownLatch initLatch = new CountDownLatch(1);

        ExecutionHandlers handlers =
                ExecutionHandlers.builder()
                        .onInit(
                                (ExecutionInit init) -> {
                                    initEvents.add(init);
                                    initLatch.countDown();
                                })
                        .onExecutionComplete(completedEvents::add)
                        .onError(errors::add)
                        .build();

        ExecutorService ex = Executors.newSingleThreadExecutor();
        long start = System.currentTimeMillis();
        Future<Execution> future =
                ex.submit(
                        () ->
                                sandbox.commands()
                                        .run(
                                                RunCommandRequest.builder()
                                                        .command("sleep 30")
                                                        .handlers(handlers)
                                                        .build()));
        assertTrue(initLatch.await(15, TimeUnit.SECONDS), "did not receive init event");
        assertEquals(1, initEvents.size());
        String id = initEvents.get(0).getId();
        assertNotNull(id);
        Thread.sleep(2000);
        sandbox.commands().interrupt(id);
        Execution result = future.get(30, TimeUnit.SECONDS);
        long elapsed = System.currentTimeMillis() - start;
        assertNotNull(result);
        assertEquals(id, result.getId());
        assertTrue(elapsed < 20_000, "Interrupted command took too long: " + elapsed + "ms");
        assertTrue((!completedEvents.isEmpty()) ^ (!errors.isEmpty()));
        assertTrue(result.getError() != null || !result.getLogs().getStderr().isEmpty());
        ex.shutdownNow();
    }

    @Test
    @Order(7)
    @DisplayName("Sandbox Pause Operation")
    @Timeout(value = 5, unit = TimeUnit.MINUTES)
    void testSandboxPause() throws InterruptedException {
        assertNotNull(sandbox);

        Thread.sleep(20000);
        sandbox.pause();

        int pollCount = 0;
        SandboxStatus finalStatus = null;

        while (pollCount < 300) {
            Thread.sleep(1000);
            pollCount++;

            SandboxInfo info = sandbox.getInfo();
            SandboxStatus currentStatus = info.getStatus();
            if ("Pausing".equals(currentStatus.getState())) {
                continue;
            }
            finalStatus = currentStatus;
            break;
        }

        assertNotNull(finalStatus, "Failed to get final status after resume operation");
        assertEquals("Paused", finalStatus.getState(), "Sandbox should be in Paused state");

        // pause => unhealthy
        boolean healthy = true;
        for (int i = 0; i < 10; i++) {
            healthy = sandbox.isHealthy();
            if (!healthy) break;
            Thread.sleep(500);
        }
        assertFalse(healthy, "Sandbox should be unhealthy after pause");
    }

    @Test
    @Order(8)
    @DisplayName("Sandbox Resume Operation")
    @Timeout(value = 3, unit = TimeUnit.MINUTES)
    void testSandboxResume() throws InterruptedException {
        assertNotNull(sandbox);

        Sandbox resumedSandbox =
                Sandbox.resumer()
                        .sandboxId(sandbox.getId())
                        .connectionConfig(sharedConnectionConfig)
                        .resumeTimeout(Duration.ofMinutes(1))
                        .healthCheckPollingInterval(Duration.ofSeconds(1))
                        .resume();

        SandboxStatus status = resumedSandbox.getInfo().getStatus();

        assertNotNull(status, "Failed to get final status after resume operation");
        assertEquals("Running", status.getState());

        boolean healthy = false;
        for (int i = 0; i < 30; i++) {
            healthy = sandbox.isHealthy();
            if (healthy) break;
            Thread.sleep(1000);
        }
        assertTrue(healthy, "Sandbox should be healthy after resume");
    }
}

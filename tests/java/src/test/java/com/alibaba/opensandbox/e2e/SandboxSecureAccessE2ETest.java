/*
 * Copyright 2026 Alibaba Group Holding Ltd.
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
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.Execution;
import com.alibaba.opensandbox.sandbox.domain.models.execd.executions.RunCommandRequest;
import com.alibaba.opensandbox.sandbox.domain.models.sandboxes.SandboxEndpoint;
import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

@Tag("e2e")
@DisplayName("Sandbox secured access E2E Tests (Java SDK)")
public class SandboxSecureAccessE2ETest extends BaseE2ETest {
    private static final String EXECD_ACCESS_TOKEN_HEADER = "X-EXECD-ACCESS-TOKEN";

    @Test
    @DisplayName("secureAccess protects execd and SDK sends endpoint access token")
    @Timeout(value = 3, unit = TimeUnit.MINUTES)
    void testSecureAccessTokenEndToEnd() throws Exception {
        Sandbox sandbox =
                Sandbox.builder()
                        .connectionConfig(sharedConnectionConfig)
                        .image(getSandboxImage())
                        .timeout(Duration.ofMinutes(2))
                        .readyTimeout(Duration.ofSeconds(60))
                        .secureAccess()
                        .metadata(Map.of("tag", "secure-access-java-e2e-test"))
                        .build();

        try {
            SandboxEndpoint execdEndpoint = sandbox.getEndpoint(44772);
            assertEndpointHasPort(execdEndpoint.getEndpoint(), 44772);

            Map<String, String> execdHeaders = execdEndpoint.getHeaders();
            assertNotNull(execdHeaders);
            assertTrue(
                    execdHeaders.containsKey(EXECD_ACCESS_TOKEN_HEADER),
                    "secureAccess endpoint must include execd access token header");
            String token = execdHeaders.get(EXECD_ACCESS_TOKEN_HEADER);
            assertNotNull(token);
            assertFalse(token.isBlank());

            SandboxEndpoint userEndpoint = sandbox.getEndpoint(8080);
            assertEquals(
                    token,
                    userEndpoint.getHeaders().get(EXECD_ACCESS_TOKEN_HEADER),
                    "all endpoints for a secured sandbox should return the execd access token");

            Execution sdkRun =
                    sandbox.commands()
                            .run(
                                    RunCommandRequest.builder()
                                            .command("echo secure-access-sdk-ok")
                                            .build());
            assertNotNull(sdkRun);
            assertNull(sdkRun.getError(), "SDK command should include endpoint headers");
            assertEquals(1, sdkRun.getLogs().getStdout().size());
            assertEquals("secure-access-sdk-ok", sdkRun.getLogs().getStdout().get(0).getText());

            HttpClient client =
                    HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(10)).build();
            URI pingUri = endpointUri(execdEndpoint, "/ping");

            HttpResponse<String> missingToken = sendPing(client, pingUri, execdHeaders, null);
            assertEquals(401, missingToken.statusCode(), "execd must reject missing access token");

            HttpResponse<String> wrongToken =
                    sendPing(client, pingUri, execdHeaders, "definitely-wrong-token");
            assertEquals(401, wrongToken.statusCode(), "execd must reject wrong access token");

            HttpResponse<String> correctToken = sendPing(client, pingUri, execdHeaders, token);
            assertEquals(200, correctToken.statusCode(), "execd must accept the endpoint token");
        } finally {
            killAndClose(sandbox);
        }
    }

    @Test
    @DisplayName("default sandbox does not require execd access token")
    @Timeout(value = 3, unit = TimeUnit.MINUTES)
    void testDefaultSandboxDoesNotReturnAccessToken() throws Exception {
        Sandbox sandbox =
                Sandbox.builder()
                        .connectionConfig(sharedConnectionConfig)
                        .image(getSandboxImage())
                        .timeout(Duration.ofMinutes(2))
                        .readyTimeout(Duration.ofSeconds(60))
                        .metadata(Map.of("tag", "non-secure-access-java-e2e-test"))
                        .build();

        try {
            SandboxEndpoint execdEndpoint = sandbox.getEndpoint(44772);
            assertEndpointHasPort(execdEndpoint.getEndpoint(), 44772);
            assertFalse(
                    execdEndpoint.getHeaders().containsKey(EXECD_ACCESS_TOKEN_HEADER),
                    "default sandbox endpoint should not include execd access token");

            HttpClient client =
                    HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(10)).build();
            HttpResponse<String> ping =
                    sendPing(client, endpointUri(execdEndpoint, "/ping"), execdEndpoint.getHeaders(), null);
            assertEquals(200, ping.statusCode(), "default execd should allow requests without token");
        } finally {
            killAndClose(sandbox);
        }
    }

    private static URI endpointUri(SandboxEndpoint endpoint, String path) {
        String protocol = testProperties.getProperty("opensandbox.test.protocol", "https");
        String base = endpoint.getEndpoint();
        while (base.endsWith("/")) {
            base = base.substring(0, base.length() - 1);
        }
        String normalizedPath = path.startsWith("/") ? path : "/" + path;
        return URI.create(protocol + "://" + base + normalizedPath);
    }

    private static HttpResponse<String> sendPing(
            HttpClient client, URI uri, Map<String, String> endpointHeaders, String token)
            throws IOException, InterruptedException {
        HttpRequest.Builder builder = HttpRequest.newBuilder(uri).timeout(Duration.ofSeconds(20)).GET();
        for (Map.Entry<String, String> header : endpointHeaders.entrySet()) {
            if (!EXECD_ACCESS_TOKEN_HEADER.equalsIgnoreCase(header.getKey())) {
                builder.header(header.getKey(), header.getValue());
            }
        }
        if (token != null) {
            builder.header(EXECD_ACCESS_TOKEN_HEADER, token);
        }
        return client.send(builder.build(), HttpResponse.BodyHandlers.ofString());
    }

    private static void killAndClose(Sandbox sandbox) {
        if (sandbox == null) {
            return;
        }
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

// Copyright 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

using System.Net;
using System.Text;
using FluentAssertions;
using OpenSandbox.Adapters;
using OpenSandbox.Internal;
using OpenSandbox.Models;
using Xunit;

namespace OpenSandbox.Tests;

public class SandboxesAdapterTests
{
    [Fact]
    public async Task GetSandboxEndpointAsync_ShouldIncludeUseServerProxyQueryParam()
    {
        // Arrange
        var handler = new CaptureHandler();
        var client = new HttpClient(handler);
        var wrapper = new HttpClientWrapper(client, "http://localhost:8080/v1");
        var adapter = new SandboxesAdapter(wrapper);

        // Act
        _ = await adapter.GetSandboxEndpointAsync("sbx-1", 44772, useServerProxy: true);

        // Assert
        handler.LastRequestUri.Should().NotBeNull();
        handler.LastRequestUri!.PathAndQuery.Should().Contain("/sandboxes/sbx-1/endpoints/44772");
        handler.LastRequestUri!.Query.Should().Contain("use_server_proxy=true");
    }

    [Fact]
    public async Task GetSandboxEndpointAsync_ShouldDefaultUseServerProxyToFalse()
    {
        // Arrange
        var handler = new CaptureHandler();
        var client = new HttpClient(handler);
        var wrapper = new HttpClientWrapper(client, "http://localhost:8080/v1");
        var adapter = new SandboxesAdapter(wrapper);

        // Act
        _ = await adapter.GetSandboxEndpointAsync("sbx-2", 44772);

        // Assert
        handler.LastRequestUri.Should().NotBeNull();
        handler.LastRequestUri!.Query.Should().Contain("use_server_proxy=false");
    }

    [Fact]
    public async Task GetSandboxAsync_ShouldTreatMissingExpiresAtAsNull()
    {
        var payload = """
        {
          "id": "sbx-1",
          "image": { "uri": "python:3.11" },
          "entrypoint": ["python"],
          "status": { "state": "Running" },
          "createdAt": "2026-03-14T12:00:00Z"
        }
        """;
        var adapter = CreateAdapterWithJsonResponse(payload);

        SandboxInfo sandbox = await adapter.GetSandboxAsync("sbx-1");

        sandbox.ExpiresAt.Should().BeNull();
    }

    [Fact]
    public async Task CreateSandboxAsync_ShouldTreatMissingExpiresAtAsNull()
    {
        var payload = """
        {
          "id": "sbx-2",
          "status": { "state": "Pending" },
          "createdAt": "2026-03-14T12:00:00Z",
          "entrypoint": ["python"]
        }
        """;
        var adapter = CreateAdapterWithJsonResponse(payload);

        CreateSandboxResponse response = await adapter.CreateSandboxAsync(new CreateSandboxRequest
        {
            Image = new ImageSpec { Uri = "python:3.11" },
            ResourceLimits = new Dictionary<string, string>(),
            Entrypoint = new List<string> { "python" }
        });

        response.ExpiresAt.Should().BeNull();
    }

    private static SandboxesAdapter CreateAdapterWithJsonResponse(string payload)
    {
        var handler = new StaticJsonHandler(payload);
        var client = new HttpClient(handler);
        var wrapper = new HttpClientWrapper(client, "http://localhost:8080/v1");
        return new SandboxesAdapter(wrapper);
    }

    private sealed class CaptureHandler : HttpMessageHandler
    {
        public Uri? LastRequestUri { get; private set; }

        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        {
            LastRequestUri = request.RequestUri;
            var payload = "{\"endpoint\":\"example.internal:44772\",\"headers\":{}}";
            var response = new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent(payload, Encoding.UTF8, "application/json")
            };
            return Task.FromResult(response);
        }
    }

    private sealed class StaticJsonHandler(string payload) : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        {
            var response = new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent(payload, Encoding.UTF8, "application/json")
            };
            return Task.FromResult(response);
        }
    }
}

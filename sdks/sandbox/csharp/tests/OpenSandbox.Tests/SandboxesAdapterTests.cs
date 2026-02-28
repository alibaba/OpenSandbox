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
}

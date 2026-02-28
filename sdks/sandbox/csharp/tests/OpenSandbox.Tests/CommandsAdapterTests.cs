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
using System.Text.Json;
using FluentAssertions;
using OpenSandbox.Adapters;
using OpenSandbox.Internal;
using OpenSandbox.Models;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;
using Xunit;

namespace OpenSandbox.Tests;

public class CommandsAdapterTests
{
    [Fact]
    public async Task GetCommandStatusAsync_ShouldParseStatusResponse()
    {
        var httpHandler = new StubHttpMessageHandler((request, _) =>
        {
            var body = "{\"id\":\"cmd-1\",\"content\":\"sleep 1\",\"running\":true,\"exit_code\":null}";
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent(body, Encoding.UTF8, "application/json")
            });
        });
        var adapter = CreateAdapter(httpHandler);

        var status = await adapter.GetCommandStatusAsync("cmd-1");

        status.Id.Should().Be("cmd-1");
        status.Content.Should().Be("sleep 1");
        status.Running.Should().BeTrue();
        status.ExitCode.Should().BeNull();
        httpHandler.RequestUris.Should().Contain(uri => uri.EndsWith("/command/status/cmd-1", StringComparison.Ordinal));
    }

    [Fact]
    public async Task GetBackgroundCommandLogsAsync_ShouldParseCursorHeader()
    {
        var httpHandler = new StubHttpMessageHandler((request, _) =>
        {
            var response = new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("line1\nline2\n", Encoding.UTF8, "text/plain")
            };
            response.Headers.Add("EXECD-COMMANDS-TAIL-CURSOR", "42");
            return Task.FromResult(response);
        });
        var adapter = CreateAdapter(httpHandler);

        var logs = await adapter.GetBackgroundCommandLogsAsync("cmd-2", cursor: 10);

        logs.Content.Should().Contain("line1");
        logs.Cursor.Should().Be(42);
        httpHandler.RequestUris.Should().Contain(uri => uri.Contains("/command/cmd-2/logs?cursor=10", StringComparison.Ordinal));
    }

    [Fact]
    public async Task GetBackgroundCommandLogsAsync_ShouldReturnNullCursorWhenHeaderMissing()
    {
        var httpHandler = new StubHttpMessageHandler((request, _) =>
        {
            return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("only-content", Encoding.UTF8, "text/plain")
            });
        });
        var adapter = CreateAdapter(httpHandler);

        var logs = await adapter.GetBackgroundCommandLogsAsync("cmd-3");

        logs.Content.Should().Be("only-content");
        logs.Cursor.Should().BeNull();
    }

    [Fact]
    public async Task RunStreamAsync_ShouldSendTimeoutInMilliseconds()
    {
        var handler = new StubHttpMessageHandler(async (request, _) =>
        {
            request.Content.Should().NotBeNull();
            var body = await request.Content!.ReadAsStringAsync().ConfigureAwait(false);
            using var doc = JsonDocument.Parse(body);
            doc.RootElement.GetProperty("timeout").GetInt64().Should().Be(2000);

            return new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent("data: {\"type\":\"init\",\"text\":\"cmd-1\"}\n\n", Encoding.UTF8, "text/event-stream")
            };
        });
        var adapter = CreateAdapter(handler);

        var options = new RunCommandOptions
        {
            TimeoutSeconds = 2
        };

        await foreach (var _ in adapter.RunStreamAsync("sleep 1", options))
        {
            // Drain events.
        }
    }

    private static CommandsAdapter CreateAdapter(HttpMessageHandler httpHandler)
    {
        var baseUrl = "http://execd.local";
        var headers = new Dictionary<string, string> { ["X-Test"] = "true" };
        var client = new HttpClientWrapper(new HttpClient(httpHandler), baseUrl, headers);
        var sseHttpClient = new HttpClient(httpHandler);
        var logger = NullLoggerFactory.Instance.CreateLogger("CommandsAdapterTests");
        return new CommandsAdapter(client, sseHttpClient, baseUrl, headers, logger);
    }

    private sealed class StubHttpMessageHandler : HttpMessageHandler
    {
        private readonly Func<HttpRequestMessage, CancellationToken, Task<HttpResponseMessage>> _handler;

        public StubHttpMessageHandler(Func<HttpRequestMessage, CancellationToken, Task<HttpResponseMessage>> handler)
        {
            _handler = handler;
        }

        public List<string> RequestUris { get; } = new();

        protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request, CancellationToken cancellationToken)
        {
            RequestUris.Add(request.RequestUri?.ToString() ?? string.Empty);
            return await _handler(request, cancellationToken).ConfigureAwait(false);
        }
    }
}

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

using System.Runtime.CompilerServices;
using System.Text;
using System.Text.Json;
using OpenSandbox.Internal;
using OpenSandbox.Models;
using OpenSandbox.Services;

namespace OpenSandbox.Adapters;

/// <summary>
/// Adapter for the execd commands service.
/// </summary>
internal sealed class CommandsAdapter : IExecdCommands
{
    private readonly HttpClientWrapper _client;
    private readonly HttpClient _sseHttpClient;
    private readonly string _baseUrl;
    private readonly IReadOnlyDictionary<string, string> _headers;

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.WhenWritingNull
    };

    public CommandsAdapter(
        HttpClientWrapper client,
        HttpClient sseHttpClient,
        string baseUrl,
        IReadOnlyDictionary<string, string> headers)
    {
        _client = client ?? throw new ArgumentNullException(nameof(client));
        _sseHttpClient = sseHttpClient ?? throw new ArgumentNullException(nameof(sseHttpClient));
        _baseUrl = baseUrl?.TrimEnd('/') ?? throw new ArgumentNullException(nameof(baseUrl));
        _headers = headers ?? new Dictionary<string, string>();
    }

    public async IAsyncEnumerable<ServerStreamEvent> RunStreamAsync(
        string command,
        RunCommandOptions? options = null,
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        var url = $"{_baseUrl}/command";
        var requestBody = new RunCommandRequest
        {
            Command = command,
            Cwd = options?.WorkingDirectory,
            Background = options?.Background
        };

        var json = JsonSerializer.Serialize(requestBody, JsonOptions);
        using var request = new HttpRequestMessage(HttpMethod.Post, url)
        {
            Content = new StringContent(json, Encoding.UTF8, "application/json")
        };

        request.Headers.Accept.Add(new System.Net.Http.Headers.MediaTypeWithQualityHeaderValue("text/event-stream"));

        foreach (var header in _headers)
        {
            request.Headers.TryAddWithoutValidation(header.Key, header.Value);
        }

        var response = await _sseHttpClient.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, cancellationToken).ConfigureAwait(false);

        await foreach (var ev in SseParser.ParseJsonEventStreamAsync<ServerStreamEvent>(response, "Run command failed", cancellationToken).ConfigureAwait(false))
        {
            yield return ev;
        }
    }

    public async Task<Execution> RunAsync(
        string command,
        RunCommandOptions? options = null,
        ExecutionHandlers? handlers = null,
        CancellationToken cancellationToken = default)
    {
        var execution = new Execution();
        var dispatcher = new ExecutionEventDispatcher(execution, handlers);

        await foreach (var ev in RunStreamAsync(command, options, cancellationToken).ConfigureAwait(false))
        {
            // Keep legacy behavior: if server sends "init" with empty id, preserve previous id
            if (ev.Type == ServerStreamEventTypes.Init && string.IsNullOrEmpty(ev.Text) && !string.IsNullOrEmpty(execution.Id))
            {
                ev.Text = execution.Id;
            }

            await dispatcher.DispatchAsync(ev).ConfigureAwait(false);
        }

        return execution;
    }

    public async Task InterruptAsync(string sessionId, CancellationToken cancellationToken = default)
    {
        var queryParams = new Dictionary<string, string?> { ["id"] = sessionId };
        await _client.DeleteAsync("/command", queryParams, cancellationToken).ConfigureAwait(false);
    }
}

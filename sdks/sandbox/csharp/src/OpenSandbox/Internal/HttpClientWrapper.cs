// Copyright 2026 The OpenSandbox Authors
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

using System.Diagnostics;
using System.Text;
using System.Text.Json;
using OpenSandbox.Config;
using OpenSandbox.Core;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace OpenSandbox.Internal;

/// <summary>
/// Internal HTTP client wrapper for making API requests.
/// </summary>
internal sealed class HttpClientWrapper
{
    private static readonly HttpMethod PatchMethod = new("PATCH");

    private readonly HttpClient _httpClient;
    private readonly string _baseUrl;
    private readonly IReadOnlyDictionary<string, string> _defaultHeaders;
    private readonly ILogger _logger;
    private readonly WakeRetryOptions _wakeRetry;

    /// <summary>
    /// The gateway emits <c>Retry-After: 1</c> on wake-timeout responses; used
    /// as the fallback wait when a marked response arrives without the header.
    /// </summary>
    private static readonly TimeSpan WakeDefaultRetryAfter = TimeSpan.FromSeconds(1);

    /// <summary>
    /// Fixed operational ceiling for server-supplied <c>Retry-After</c> waits.
    /// Not configurable: ignoring a server's back-pressure signal is never a
    /// safe default, and the 60s guard only defends against a pathological
    /// header.
    /// </summary>
    private static readonly TimeSpan RetryAfterCap = TimeSpan.FromSeconds(60);

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        PropertyNameCaseInsensitive = true,
        DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.WhenWritingNull
    };

    public HttpClientWrapper(
        HttpClient httpClient,
        string baseUrl,
        IReadOnlyDictionary<string, string>? defaultHeaders = null,
        ILogger? logger = null,
        WakeRetryOptions? wakeRetry = null)
    {
        _httpClient = httpClient ?? throw new ArgumentNullException(nameof(httpClient));
        _baseUrl = baseUrl?.TrimEnd('/') ?? throw new ArgumentNullException(nameof(baseUrl));
        _defaultHeaders = defaultHeaders ?? new Dictionary<string, string>();
        _logger = logger ?? NullLogger.Instance;
        _wakeRetry = wakeRetry ?? new WakeRetryOptions(true, 5, TimeSpan.FromSeconds(60));
    }

    public string BaseUrl => _baseUrl;

    public HttpClientWrapper WithBaseUrl(string baseUrl) =>
        new(_httpClient, baseUrl, _defaultHeaders, _logger);

    public async Task<T> GetAsync<T>(
        string path,
        Dictionary<string, string?>? queryParams = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path, queryParams);
        _logger.LogDebug("HTTP GET {Url}", url);
        using var response = await SendWithWakeRetryAsync(
            () =>
            {
                var request = new HttpRequestMessage(HttpMethod.Get, url);
                ApplyDefaultHeaders(request);
                return request;
            },
            cancellationToken).ConfigureAwait(false);
        return await HandleResponseAsync<T>(response, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Gets a JSON resource and also returns response headers, e.g. to read
    /// metadata such as the OPEN-SANDBOX-ORIGIN header.
    /// </summary>
    public async Task<(T Body, IReadOnlyDictionary<string, string> Headers)> GetWithHeadersAsync<T>(
        string path,
        Dictionary<string, string?>? queryParams = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path, queryParams);
        _logger.LogDebug("HTTP GET {Url}", url);
        using var response = await SendWithWakeRetryAsync(
            () =>
            {
                var request = new HttpRequestMessage(HttpMethod.Get, url);
                ApplyDefaultHeaders(request);
                return request;
            },
            cancellationToken).ConfigureAwait(false);
        var headers = response.Headers.ToDictionary(h => h.Key, h => h.Value.FirstOrDefault() ?? string.Empty, StringComparer.OrdinalIgnoreCase);
        var body = await HandleResponseAsync<T>(response, cancellationToken).ConfigureAwait(false);
        return (body, headers);
    }

    public async Task GetAsync(
        string path,
        Dictionary<string, string?>? queryParams = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path, queryParams);
        _logger.LogDebug("HTTP GET {Url}", url);
        using var response = await SendWithWakeRetryAsync(
            () =>
            {
                var request = new HttpRequestMessage(HttpMethod.Get, url);
                ApplyDefaultHeaders(request);
                return request;
            },
            cancellationToken).ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
    }

    public async Task<T> PostAsync<T>(
        string path,
        object? body = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path);
        _logger.LogDebug("HTTP POST {Url}", url);
        using var response = await SendWithWakeRetryAsync(() => BuildJsonRequest(HttpMethod.Post, url, body), cancellationToken).ConfigureAwait(false);
        return await HandleResponseAsync<T>(response, cancellationToken).ConfigureAwait(false);
    }

    public async Task PostAsync(
        string path,
        object? body = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path);
        _logger.LogDebug("HTTP POST {Url}", url);
        using var response = await SendWithWakeRetryAsync(() => BuildJsonRequest(HttpMethod.Post, url, body), cancellationToken).ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
    }

    public async Task<T> PatchAsync<T>(
        string path,
        object? body = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path);
        _logger.LogDebug("HTTP PATCH {Url}", url);
        using var response = await SendWithWakeRetryAsync(() => BuildJsonRequest(PatchMethod, url, body), cancellationToken).ConfigureAwait(false);
        return await HandleResponseAsync<T>(response, cancellationToken).ConfigureAwait(false);
    }

    public async Task PatchAsync(
        string path,
        object? body = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path);
        _logger.LogDebug("HTTP PATCH {Url}", url);
        using var response = await SendWithWakeRetryAsync(() => BuildJsonRequest(PatchMethod, url, body), cancellationToken).ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
    }

    public async Task<T> DeleteAsync<T>(
        string path,
        Dictionary<string, string?>? queryParams = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path, queryParams);
        _logger.LogDebug("HTTP DELETE {Url}", url);
        using var response = await SendWithWakeRetryAsync(
            () =>
            {
                var request = new HttpRequestMessage(HttpMethod.Delete, url);
                ApplyDefaultHeaders(request);
                return request;
            },
            cancellationToken).ConfigureAwait(false);
        return await HandleResponseAsync<T>(response, cancellationToken).ConfigureAwait(false);
    }

    public async Task DeleteAsync(
        string path,
        Dictionary<string, string?>? queryParams = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path, queryParams);
        _logger.LogDebug("HTTP DELETE {Url}", url);
        using var response = await SendWithWakeRetryAsync(
            () =>
            {
                var request = new HttpRequestMessage(HttpMethod.Delete, url);
                ApplyDefaultHeaders(request);
                return request;
            },
            cancellationToken).ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
    }

    public async Task DeleteAsync(
        string path,
        object body,
        CancellationToken cancellationToken)
    {
        var url = BuildUrl(path);
        _logger.LogDebug("HTTP DELETE {Url}", url);
        using var response = await SendWithWakeRetryAsync(() => BuildJsonRequest(HttpMethod.Delete, url, body), cancellationToken).ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
    }

    public async Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage request,
        CancellationToken cancellationToken = default)
    {
        _logger.LogDebug("HTTP {Method} {Url}", request.Method, request.RequestUri);
        ApplyDefaultHeaders(request);
        return await _httpClient.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, cancellationToken).ConfigureAwait(false);
    }

    public async Task<byte[]> GetBytesAsync(
        string path,
        Dictionary<string, string?>? queryParams = null,
        Dictionary<string, string>? headers = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path, queryParams);
        using var request = new HttpRequestMessage(HttpMethod.Get, url);
        ApplyDefaultHeaders(request);

        if (headers != null)
        {
            foreach (var header in headers)
            {
                request.Headers.TryAddWithoutValidation(header.Key, header.Value);
            }
        }

        using var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
        return await response.Content.ReadAsByteArrayAsync().ConfigureAwait(false);
    }

    public async Task<Stream> GetStreamAsync(
        string path,
        Dictionary<string, string?>? queryParams = null,
        Dictionary<string, string>? headers = null,
        CancellationToken cancellationToken = default)
    {
        var url = BuildUrl(path, queryParams);
        using var request = new HttpRequestMessage(HttpMethod.Get, url);
        ApplyDefaultHeaders(request);

        if (headers != null)
        {
            foreach (var header in headers)
            {
                request.Headers.TryAddWithoutValidation(header.Key, header.Value);
            }
        }

        var response = await _httpClient.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, cancellationToken).ConfigureAwait(false);
        await EnsureSuccessAsync(response, cancellationToken).ConfigureAwait(false);
        return await response.Content.ReadAsStreamAsync().ConfigureAwait(false);
    }

    private HttpRequestMessage BuildJsonRequest(HttpMethod method, string url, object? body)
    {
        var request = new HttpRequestMessage(method, url);
        ApplyDefaultHeaders(request);
        if (body != null)
        {
            var json = JsonSerializer.Serialize(body, JsonOptions);
            request.Content = new StringContent(json, Encoding.UTF8, "application/json");
        }
        return request;
    }

    /// <summary>
    /// Whether the response is the OSEP-0024 wake-timeout answer.
    /// <para>
    /// Recognition is header-only on a 503: the gateway strips
    /// <c>OpenSandbox-Wake</c> from every upstream response, so a matching
    /// marker proves the gateway parked the request and never forwarded it.
    /// The JSON <c>code</c> in the body is documentation for humans, not a
    /// recognition signal — a forged body without the header follows the
    /// SDK's existing (bare 503) rules untouched.
    /// </para>
    /// </summary>
    private static bool IsWakeTimeoutResponse(HttpResponseMessage response)
    {
        return (int)response.StatusCode == 503 &&
               response.Headers.TryGetValues(Constants.WakeMarkerHeader, out var values) &&
               string.Equals(values.FirstOrDefault(), Constants.WakeMarkerExpired, StringComparison.OrdinalIgnoreCase);
    }

    /// <summary>
    /// Effective wake retry wait for a marked response: the parsed
    /// <c>Retry-After</c> (delta-seconds or HTTP-date) capped at the
    /// operational ceiling, or the 1-second fallback when the header is
    /// missing or unparseable.
    /// </summary>
    private static TimeSpan ResolveWakeRetryAfter(HttpResponseMessage response)
    {
        TimeSpan? parsed = null;
        if (response.Headers.TryGetValues("Retry-After", out var values))
        {
            var raw = values.FirstOrDefault();
            if (!string.IsNullOrWhiteSpace(raw))
            {
                var trimmed = raw.Trim();
                if (int.TryParse(trimmed, out var seconds))
                {
                    parsed = seconds <= 0 ? TimeSpan.Zero : TimeSpan.FromSeconds(seconds);
                }
                else if (DateTimeOffset.TryParse(trimmed, out var date))
                {
                    var delta = date - DateTimeOffset.UtcNow;
                    parsed = delta < TimeSpan.Zero ? TimeSpan.Zero : delta;
                }
            }
        }

        var wait = parsed ?? WakeDefaultRetryAfter;
        return wait > RetryAfterCap ? RetryAfterCap : wait;
    }

    /// <summary>
    /// Sends a JSON request, applying the OSEP-0024 wake retry contract.
    /// <para>
    /// On a marked wake-timeout response the original request is rebuilt and
    /// resent after waiting per <c>Retry-After</c> (any method — JSON request
    /// bodies are re-serializable, so replay is always safe), until the
    /// sandbox is <c>Running</c> or the policy budget is exhausted;
    /// exhaustion throws <see cref="SandboxWakeTimeoutException"/> while the
    /// server-side resume continues in the background. Every other response —
    /// including a bare 503 — is returned untouched.
    /// </para>
    /// </summary>
    private async Task<HttpResponseMessage> SendWithWakeRetryAsync(
        Func<HttpRequestMessage> requestFactory,
        CancellationToken cancellationToken)
    {
        if (!_wakeRetry.Enabled)
        {
            using var singleRequest = requestFactory();
            return await _httpClient.SendAsync(singleRequest, cancellationToken).ConfigureAwait(false);
        }

        var stopwatch = Stopwatch.StartNew();
        var attempts = 0;
        while (true)
        {
            using var request = requestFactory();
            var response = await _httpClient.SendAsync(request, cancellationToken).ConfigureAwait(false);
            if (!IsWakeTimeoutResponse(response))
            {
                return response;
            }

            attempts++;
            var elapsed = stopwatch.Elapsed;
            var retryAfter = ResolveWakeRetryAfter(response);
            // Budget checks bound retries, not a committed resume: the
            // server-side restore is never canceled by giving up here.
            var exhausted = attempts >= _wakeRetry.MaxAttempts ||
                            (_wakeRetry.OverallDeadline.HasValue &&
                             retryAfter > _wakeRetry.OverallDeadline.Value - elapsed);
            if (exhausted)
            {
                var content = await response.Content.ReadAsStringAsync().ConfigureAwait(false);
                var requestId = response.Headers.TryGetValues(Constants.RequestIdHeader, out var ids)
                    ? ids.FirstOrDefault()
                    : null;
                throw new SandboxWakeTimeoutException(
                    attempts,
                    elapsed,
                    requestId: requestId,
                    rawBody: content);
            }

            _logger.LogInformation(
                "Wake retry: marked 503 received; restore still in progress (attempt={Attempt}, retryAfter={RetryAfterMs}ms)",
                attempts,
                retryAfter.TotalMilliseconds);
            response.Dispose();
            await Task.Delay(retryAfter, cancellationToken).ConfigureAwait(false);
        }
    }

    private string BuildUrl(string path, Dictionary<string, string?>? queryParams = null)
    {
        var url = path.StartsWith("/") ? $"{_baseUrl}{path}" : $"{_baseUrl}/{path}";

        if (queryParams == null || queryParams.Count == 0)
            return url;

        var queryString = string.Join("&",
            queryParams
                .Where(kv => kv.Value != null)
                .Select(kv => $"{Uri.EscapeDataString(kv.Key)}={Uri.EscapeDataString(kv.Value!)}"));

        return string.IsNullOrEmpty(queryString) ? url : $"{url}?{queryString}";
    }

    private void ApplyDefaultHeaders(HttpRequestMessage request)
    {
        foreach (var header in _defaultHeaders)
        {
            if (!request.Headers.Contains(header.Key))
            {
                request.Headers.TryAddWithoutValidation(header.Key, header.Value);
            }
        }
    }

    private async Task<T> HandleResponseAsync<T>(HttpResponseMessage response, CancellationToken cancellationToken)
    {
        var content = await response.Content.ReadAsStringAsync().ConfigureAwait(false);

        if (!response.IsSuccessStatusCode)
        {
            LogHttpFailure(response);
            ThrowApiException(response, content);
        }

        if (string.IsNullOrEmpty(content))
        {
            throw new SandboxApiException(
                message: "Unexpected empty response body",
                statusCode: (int)response.StatusCode,
                error: new SandboxError(SandboxErrorCodes.UnexpectedResponse, "Unexpected empty response body"),
                rawBody: content);
        }

        try
        {
            return JsonSerializer.Deserialize<T>(content, JsonOptions)!;
        }
        catch (JsonException ex)
        {
            throw new SandboxApiException(
                message: $"Failed to deserialize response: {ex.Message}",
                statusCode: (int)response.StatusCode,
                rawBody: content,
                innerException: ex);
        }
    }

    private async Task EnsureSuccessAsync(HttpResponseMessage response, CancellationToken cancellationToken)
    {
        if (!response.IsSuccessStatusCode)
        {
            var content = await response.Content.ReadAsStringAsync().ConfigureAwait(false);
            LogHttpFailure(response);
            ThrowApiException(response, content);
        }
    }

    private void LogHttpFailure(HttpResponseMessage response)
    {
        var request = response.RequestMessage;
        var requestId = response.Headers.TryGetValues(Constants.RequestIdHeader, out var values)
            ? values.FirstOrDefault()
            : null;

        _logger.LogError(
            "HTTP request failed: method={Method}, url={Url}, status={StatusCode}, requestId={RequestId}",
            request?.Method.Method ?? "UNKNOWN",
            request?.RequestUri?.ToString() ?? "UNKNOWN",
            (int)response.StatusCode,
            requestId ?? string.Empty);
    }

    private static void ThrowApiException(HttpResponseMessage response, string content)
    {
        var requestId = response.Headers.TryGetValues(Constants.RequestIdHeader, out var values)
            ? values.FirstOrDefault()
            : null;

        string? errorMessage = null;
        string? errorCode = null;
        object? rawBody = content;

        if (!string.IsNullOrEmpty(content))
        {
            try
            {
                var parsed = JsonSerializer.Deserialize<Dictionary<string, JsonElement>>(content, JsonOptions);
                if (parsed != null)
                {
                    rawBody = parsed;
                    if (parsed.TryGetValue("message", out var msg))
                        errorMessage = msg.GetString();
                    if (parsed.TryGetValue("error", out var err) && err.ValueKind == JsonValueKind.Object)
                    {
                        if (err.TryGetProperty("message", out var errMsg))
                            errorMessage = errorMessage ?? errMsg.GetString();
                        if (err.TryGetProperty("code", out var errCode))
                            errorCode = errCode.GetString();
                    }
                    if (parsed.TryGetValue("code", out var code))
                        errorCode = errorCode ?? code.GetString();
                }
            }
            catch
            {
                // Ignore JSON parse errors
            }
        }

        var message = errorMessage ?? $"Request failed with status code {(int)response.StatusCode}" +
            (string.IsNullOrEmpty(content) ? string.Empty : $": {content}");
        var sandboxErrorCode = errorCode ?? SandboxErrorCodes.UnexpectedResponse;

        throw new SandboxApiException(
            message: message,
            statusCode: (int)response.StatusCode,
            requestId: requestId,
            rawBody: rawBody,
            error: new SandboxError(sandboxErrorCode, errorMessage ?? message));
    }
}

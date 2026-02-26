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

using OpenSandbox.Config;
using OpenSandbox.Core;
using OpenSandbox.Factory;
using OpenSandbox.Models;
using OpenSandbox.Services;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace OpenSandbox;

/// <summary>
/// Main entry point for interacting with a sandbox.
/// </summary>
/// <remarks>
/// <see cref="DisposeAsync"/> releases local SDK resources (HTTP clients and adapters) only.
/// To terminate the remote sandbox instance, call <see cref="KillAsync"/>.
/// </remarks>
public sealed class Sandbox : IAsyncDisposable
{
    /// <summary>
    /// Gets the sandbox ID.
    /// </summary>
    public string Id { get; }

    /// <summary>
    /// Gets the connection configuration.
    /// </summary>
    public ConnectionConfig ConnectionConfig { get; }

    /// <summary>
    /// Gets the command execution service.
    /// </summary>
    public IExecdCommands Commands { get; }

    /// <summary>
    /// Gets the filesystem service.
    /// </summary>
    public ISandboxFiles Files { get; }

    /// <summary>
    /// Gets the health check service.
    /// </summary>
    public IExecdHealth Health { get; }

    /// <summary>
    /// Gets the metrics service.
    /// </summary>
    public IExecdMetrics Metrics { get; }

    private readonly ISandboxes _sandboxes;
    private readonly IAdapterFactory _adapterFactory;
    private readonly string _lifecycleBaseUrl;
    private readonly string _execdBaseUrl;
    private readonly HttpClientProvider _httpClientProvider;
    private readonly ILoggerFactory _loggerFactory;
    private readonly ILogger _logger;
    private bool _disposed;

    internal HttpClientProvider SharedHttpClientProvider => _httpClientProvider;
    internal ILoggerFactory SharedLoggerFactory => _loggerFactory;

    private Sandbox(
        string id,
        ConnectionConfig connectionConfig,
        IAdapterFactory adapterFactory,
        string lifecycleBaseUrl,
        string execdBaseUrl,
        ILoggerFactory loggerFactory,
        HttpClientProvider httpClientProvider,
        ISandboxes sandboxes,
        IExecdCommands commands,
        ISandboxFiles files,
        IExecdHealth health,
        IExecdMetrics metrics)
    {
        Id = id;
        ConnectionConfig = connectionConfig;
        _adapterFactory = adapterFactory;
        _lifecycleBaseUrl = lifecycleBaseUrl;
        _execdBaseUrl = execdBaseUrl;
        _loggerFactory = loggerFactory ?? NullLoggerFactory.Instance;
        _httpClientProvider = httpClientProvider;
        _logger = _loggerFactory.CreateLogger("OpenSandbox.Sandbox");
        _sandboxes = sandboxes;
        Commands = commands;
        Files = files;
        Health = health;
        Metrics = metrics;
    }

    /// <summary>
    /// Creates a new sandbox.
    /// </summary>
    /// <param name="options">The creation options.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The created sandbox.</returns>
    /// <exception cref="InvalidArgumentException">Thrown when request options are invalid.</exception>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    /// <exception cref="SandboxReadyTimeoutException">Thrown when readiness checks exceed timeout.</exception>
    /// <exception cref="SandboxException">Thrown when sandbox creation fails.</exception>
    public static async Task<Sandbox> CreateAsync(
        SandboxCreateOptions options,
        CancellationToken cancellationToken = default)
    {
        var connectionConfig = options.ConnectionConfig ?? new ConnectionConfig();
        var loggerFactory = options.Diagnostics?.LoggerFactory ?? NullLoggerFactory.Instance;
        var logger = loggerFactory.CreateLogger("OpenSandbox.Sandbox");
        var lifecycleBaseUrl = connectionConfig.GetBaseUrl();
        var adapterFactory = options.AdapterFactory ?? DefaultAdapterFactory.Create();
        var httpClientProvider = new HttpClientProvider(connectionConfig, loggerFactory);

        ISandboxes sandboxes;
        logger.LogInformation("Creating sandbox (image={Image}, useServerProxy={UseServerProxy})", options.Image, connectionConfig.UseServerProxy);
        try
        {
            var lifecycleStack = adapterFactory.CreateLifecycleStack(new CreateLifecycleStackOptions
            {
                ConnectionConfig = connectionConfig,
                LifecycleBaseUrl = lifecycleBaseUrl,
                HttpClientProvider = httpClientProvider,
                LoggerFactory = loggerFactory
            });
            sandboxes = lifecycleStack.Sandboxes;
        }
        catch
        {
            logger.LogError("Failed to initialize lifecycle adapters while creating sandbox");
            httpClientProvider.Dispose();
            throw;
        }

        var request = new CreateSandboxRequest
        {
            Image = new ImageSpec
            {
                Uri = options.Image,
                Auth = options.ImageAuth
            },
            Entrypoint = options.Entrypoint ?? Constants.DefaultEntrypoint,
            Timeout = options.TimeoutSeconds ?? Constants.DefaultTimeoutSeconds,
            ResourceLimits = options.Resource ?? Constants.DefaultResourceLimits,
            Env = options.Env,
            Metadata = options.Metadata,
            NetworkPolicy = options.NetworkPolicy != null
                ? new NetworkPolicy
                {
                    DefaultAction = options.NetworkPolicy.DefaultAction ?? NetworkRuleAction.Deny,
                    Egress = options.NetworkPolicy.Egress
                }
                : null,
            Volumes = options.Volumes,
            Extensions = options.Extensions?.ToDictionary(kv => kv.Key, kv => (object)kv.Value)
        };

        string? sandboxId = null;
        try
        {
            var created = await sandboxes.CreateSandboxAsync(request, cancellationToken).ConfigureAwait(false);
            sandboxId = created.Id;
            logger.LogInformation("Sandbox created: {SandboxId}", sandboxId);

            var endpoint = await sandboxes.GetSandboxEndpointAsync(
                sandboxId,
                Constants.DefaultExecdPort,
                connectionConfig.UseServerProxy,
                cancellationToken).ConfigureAwait(false);
            var protocol = connectionConfig.Protocol == ConnectionProtocol.Https ? "https" : "http";
            var execdBaseUrl = $"{protocol}://{endpoint.EndpointAddress}";
            var execdHeaders = MergeHeaders(connectionConfig.Headers, endpoint.Headers);

            var execdStack = adapterFactory.CreateExecdStack(new CreateExecdStackOptions
            {
                ConnectionConfig = connectionConfig,
                ExecdBaseUrl = execdBaseUrl,
                ExecdHeaders = execdHeaders,
                HttpClientProvider = httpClientProvider,
                LoggerFactory = loggerFactory
            });

            var sandbox = new Sandbox(
                sandboxId,
                connectionConfig,
                adapterFactory,
                lifecycleBaseUrl,
                execdBaseUrl,
                loggerFactory,
                httpClientProvider,
                sandboxes,
                execdStack.Commands,
                execdStack.Files,
                execdStack.Health,
                execdStack.Metrics);

            if (!options.SkipHealthCheck)
            {
                logger.LogDebug("Waiting for sandbox readiness: {SandboxId}", sandboxId);
                await sandbox.WaitUntilReadyAsync(new WaitUntilReadyOptions
                {
                    ReadyTimeoutSeconds = options.ReadyTimeoutSeconds ?? Constants.DefaultReadyTimeoutSeconds,
                    PollingIntervalMillis = options.HealthCheckPollingInterval ?? Constants.DefaultHealthCheckPollingIntervalMillis,
                    HealthCheck = options.HealthCheck
                }, cancellationToken).ConfigureAwait(false);
            }

            return sandbox;
        }
        catch (Exception ex)
        {
            if (sandboxId != null)
            {
                try
                {
                    await sandboxes.DeleteSandboxAsync(sandboxId, CancellationToken.None).ConfigureAwait(false);
                }
                catch
                {
                    // Ignore cleanup failure; surface original error
                }
            }

            httpClientProvider.Dispose();
            logger.LogError(ex, "Sandbox create flow failed");
            throw;
        }
    }

    /// <summary>
    /// Connects to an existing sandbox.
    /// </summary>
    /// <param name="options">The connection options.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The connected sandbox.</returns>
    /// <exception cref="InvalidArgumentException">Thrown when request options are invalid.</exception>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    /// <exception cref="SandboxReadyTimeoutException">Thrown when readiness checks exceed timeout.</exception>
    /// <exception cref="SandboxException">Thrown when sandbox connection fails.</exception>
    public static async Task<Sandbox> ConnectAsync(
        SandboxConnectOptions options,
        CancellationToken cancellationToken = default)
    {
        var connectionConfig = options.ConnectionConfig ?? new ConnectionConfig();
        var loggerFactory = options.Diagnostics?.LoggerFactory ?? NullLoggerFactory.Instance;
        var logger = loggerFactory.CreateLogger("OpenSandbox.Sandbox");
        var lifecycleBaseUrl = connectionConfig.GetBaseUrl();
        var adapterFactory = options.AdapterFactory ?? DefaultAdapterFactory.Create();
        var httpClientProvider = new HttpClientProvider(connectionConfig, loggerFactory);
        logger.LogInformation("Connecting to sandbox: {SandboxId}", options.SandboxId);

        ISandboxes sandboxes;
        try
        {
            var lifecycleStack = adapterFactory.CreateLifecycleStack(new CreateLifecycleStackOptions
            {
                ConnectionConfig = connectionConfig,
                LifecycleBaseUrl = lifecycleBaseUrl,
                HttpClientProvider = httpClientProvider,
                LoggerFactory = loggerFactory
            });
            sandboxes = lifecycleStack.Sandboxes;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Failed to initialize lifecycle adapters while connecting sandbox");
            httpClientProvider.Dispose();
            throw;
        }

        try
        {
            var endpoint = await sandboxes.GetSandboxEndpointAsync(
                options.SandboxId,
                Constants.DefaultExecdPort,
                connectionConfig.UseServerProxy,
                cancellationToken).ConfigureAwait(false);
            var protocol = connectionConfig.Protocol == ConnectionProtocol.Https ? "https" : "http";
            var execdBaseUrl = $"{protocol}://{endpoint.EndpointAddress}";
            var execdHeaders = MergeHeaders(connectionConfig.Headers, endpoint.Headers);

            var execdStack = adapterFactory.CreateExecdStack(new CreateExecdStackOptions
            {
                ConnectionConfig = connectionConfig,
                ExecdBaseUrl = execdBaseUrl,
                ExecdHeaders = execdHeaders,
                HttpClientProvider = httpClientProvider,
                LoggerFactory = loggerFactory
            });

            var sandbox = new Sandbox(
                options.SandboxId,
                connectionConfig,
                adapterFactory,
                lifecycleBaseUrl,
                execdBaseUrl,
                loggerFactory,
                httpClientProvider,
                sandboxes,
                execdStack.Commands,
                execdStack.Files,
                execdStack.Health,
                execdStack.Metrics);

            if (!options.SkipHealthCheck)
            {
                await sandbox.WaitUntilReadyAsync(new WaitUntilReadyOptions
                {
                    ReadyTimeoutSeconds = options.ReadyTimeoutSeconds ?? Constants.DefaultReadyTimeoutSeconds,
                    PollingIntervalMillis = options.HealthCheckPollingInterval ?? Constants.DefaultHealthCheckPollingIntervalMillis,
                    HealthCheck = options.HealthCheck
                }, cancellationToken).ConfigureAwait(false);
            }

            return sandbox;
        }
        catch (Exception ex)
        {
            logger.LogError(ex, "Sandbox connect flow failed: {SandboxId}", options.SandboxId);
            httpClientProvider.Dispose();
            throw;
        }
    }

    /// <summary>
    /// Resumes a paused sandbox by ID.
    /// </summary>
    /// <param name="options">The connection options.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The resumed sandbox.</returns>
    /// <exception cref="InvalidArgumentException">Thrown when request options are invalid.</exception>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    /// <exception cref="SandboxReadyTimeoutException">Thrown when readiness checks exceed timeout.</exception>
    /// <exception cref="SandboxException">Thrown when sandbox resume fails.</exception>
    public static async Task<Sandbox> ResumeAsync(
        SandboxConnectOptions options,
        CancellationToken cancellationToken = default)
    {
        var connectionConfig = options.ConnectionConfig ?? new ConnectionConfig();
        var loggerFactory = options.Diagnostics?.LoggerFactory ?? NullLoggerFactory.Instance;
        var logger = loggerFactory.CreateLogger("OpenSandbox.Sandbox");
        var lifecycleBaseUrl = connectionConfig.GetBaseUrl();
        var adapterFactory = options.AdapterFactory ?? DefaultAdapterFactory.Create();
        var httpClientProvider = new HttpClientProvider(connectionConfig, loggerFactory);
        logger.LogInformation("Resuming sandbox: {SandboxId}", options.SandboxId);

        try
        {
            var lifecycleStack = adapterFactory.CreateLifecycleStack(new CreateLifecycleStackOptions
            {
                ConnectionConfig = connectionConfig,
                LifecycleBaseUrl = lifecycleBaseUrl,
                HttpClientProvider = httpClientProvider,
                LoggerFactory = loggerFactory
            });

            await lifecycleStack.Sandboxes.ResumeSandboxAsync(options.SandboxId, cancellationToken).ConfigureAwait(false);
            return await ConnectAsync(options, cancellationToken).ConfigureAwait(false);
        }
        finally
        {
            httpClientProvider.Dispose();
        }
    }

    /// <summary>
    /// Gets information about this sandbox.
    /// </summary>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The sandbox information.</returns>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    public Task<SandboxInfo> GetInfoAsync(CancellationToken cancellationToken = default)
    {
        return _sandboxes.GetSandboxAsync(Id, cancellationToken);
    }

    /// <summary>
    /// Checks if the sandbox is healthy.
    /// </summary>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>True if healthy, false otherwise.</returns>
    public async Task<bool> IsHealthyAsync(CancellationToken cancellationToken = default)
    {
        try
        {
            return await Health.PingAsync(cancellationToken).ConfigureAwait(false);
        }
        catch (Exception ex)
        {
            _logger.LogDebug(ex, "Health check failed for sandbox {SandboxId}", Id);
            return false;
        }
    }

    /// <summary>
    /// Gets the current resource metrics.
    /// </summary>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The sandbox metrics.</returns>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    public Task<SandboxMetrics> GetMetricsAsync(CancellationToken cancellationToken = default)
    {
        return Metrics.GetMetricsAsync(cancellationToken);
    }

    /// <summary>
    /// Pauses the sandbox.
    /// </summary>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    public Task PauseAsync(CancellationToken cancellationToken = default)
    {
        return _sandboxes.PauseSandboxAsync(Id, cancellationToken);
    }

    /// <summary>
    /// Resumes this paused sandbox and returns a fresh, connected instance.
    /// </summary>
    /// <param name="options">Optional resume options.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>A new sandbox instance with refreshed connections.</returns>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    /// <exception cref="SandboxReadyTimeoutException">Thrown when readiness checks exceed timeout.</exception>
    /// <exception cref="SandboxException">Thrown when sandbox resume fails.</exception>
    public async Task<Sandbox> ResumeAsync(
        SandboxResumeOptions? options = null,
        CancellationToken cancellationToken = default)
    {
        await _sandboxes.ResumeSandboxAsync(Id, cancellationToken).ConfigureAwait(false);

        return await ConnectAsync(new SandboxConnectOptions
        {
            SandboxId = Id,
            ConnectionConfig = ConnectionConfig,
            Diagnostics = new SdkDiagnosticsOptions
            {
                LoggerFactory = _loggerFactory
            },
            AdapterFactory = _adapterFactory,
            SkipHealthCheck = options?.SkipHealthCheck ?? false,
            ReadyTimeoutSeconds = options?.ReadyTimeoutSeconds,
            HealthCheckPollingInterval = options?.HealthCheckPollingInterval
        }, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Terminates the sandbox.
    /// </summary>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    public Task KillAsync(CancellationToken cancellationToken = default)
    {
        return _sandboxes.DeleteSandboxAsync(Id, cancellationToken);
    }

    /// <summary>
    /// Renews the sandbox expiration time.
    /// </summary>
    /// <param name="timeoutSeconds">The new timeout in seconds from now.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The renewal response.</returns>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    public Task<RenewSandboxExpirationResponse> RenewAsync(
        int timeoutSeconds,
        CancellationToken cancellationToken = default)
    {
        var expiresAt = DateTime.UtcNow.AddSeconds(timeoutSeconds).ToString("O");
        return _sandboxes.RenewSandboxExpirationAsync(Id, new RenewSandboxExpirationRequest
        {
            ExpiresAt = expiresAt
        }, cancellationToken);
    }

    /// <summary>
    /// Gets the endpoint for a port.
    /// </summary>
    /// <param name="port">The port number.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The endpoint information.</returns>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    public Task<Endpoint> GetEndpointAsync(int port, CancellationToken cancellationToken = default)
    {
        return _sandboxes.GetSandboxEndpointAsync(Id, port, ConnectionConfig.UseServerProxy, cancellationToken);
    }

    /// <summary>
    /// Gets the endpoint URL for a port.
    /// </summary>
    /// <param name="port">The port number.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The endpoint URL.</returns>
    /// <exception cref="SandboxApiException">Thrown when the sandbox API returns an error.</exception>
    public async Task<string> GetEndpointUrlAsync(int port, CancellationToken cancellationToken = default)
    {
        var endpoint = await GetEndpointAsync(port, cancellationToken).ConfigureAwait(false);
        var protocol = ConnectionConfig.Protocol == ConnectionProtocol.Https ? "https" : "http";
        return $"{protocol}://{endpoint.EndpointAddress}";
    }

    /// <summary>
    /// Waits until the sandbox is ready.
    /// </summary>
    /// <param name="options">The wait options.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <exception cref="SandboxReadyTimeoutException">Thrown when readiness checks exceed timeout.</exception>
    /// <exception cref="OperationCanceledException">Thrown when <paramref name="cancellationToken"/> is canceled.</exception>
    public async Task WaitUntilReadyAsync(
        WaitUntilReadyOptions options,
        CancellationToken cancellationToken = default)
    {
        _logger.LogDebug("Start readiness check for sandbox {SandboxId} (timeoutSeconds={TimeoutSeconds})", Id, options.ReadyTimeoutSeconds);
        var deadline = DateTime.UtcNow.AddSeconds(options.ReadyTimeoutSeconds);

        while (true)
        {
            cancellationToken.ThrowIfCancellationRequested();

            if (DateTime.UtcNow > deadline)
            {
                throw new SandboxReadyTimeoutException(
                    $"Sandbox not ready: timed out waiting for health check (timeoutSeconds={options.ReadyTimeoutSeconds})");
            }

            try
            {
                bool isReady;
                if (options.HealthCheck != null)
                {
                    isReady = await options.HealthCheck(this).ConfigureAwait(false);
                }
                else
                {
                    isReady = await Health.PingAsync(cancellationToken).ConfigureAwait(false);
                }

                if (isReady)
                {
                    _logger.LogInformation("Sandbox is ready: {SandboxId}", Id);
                    return;
                }
            }
            catch (Exception ex)
            {
                _logger.LogDebug(ex, "Readiness probe failed for sandbox {SandboxId}", Id);
            }

            await Task.Delay(options.PollingIntervalMillis, cancellationToken).ConfigureAwait(false);
        }
    }

    /// <summary>
    /// Releases resources used by this sandbox instance.
    /// </summary>
    public ValueTask DisposeAsync()
    {
        if (_disposed)
        {
            return default;
        }

        _disposed = true;
        _logger.LogDebug("Disposing sandbox resources: {SandboxId}", Id);
        _httpClientProvider.Dispose();
        return default;
    }

    private static IReadOnlyDictionary<string, string> MergeHeaders(
        IReadOnlyDictionary<string, string> baseHeaders,
        IReadOnlyDictionary<string, string>? overrideHeaders)
    {
        var merged = baseHeaders.ToDictionary(header => header.Key, header => header.Value);
        if (overrideHeaders != null)
        {
            foreach (var header in overrideHeaders)
            {
                merged[header.Key] = header.Value;
            }
        }

        return merged;
    }
}

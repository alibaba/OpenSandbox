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

namespace OpenSandbox;

/// <summary>
/// Main entry point for interacting with a sandbox.
/// </summary>
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
    private bool _disposed;

    private Sandbox(
        string id,
        ConnectionConfig connectionConfig,
        IAdapterFactory adapterFactory,
        string lifecycleBaseUrl,
        string execdBaseUrl,
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
    public static async Task<Sandbox> CreateAsync(
        SandboxCreateOptions options,
        CancellationToken cancellationToken = default)
    {
        var connectionConfig = options.ConnectionConfig ?? new ConnectionConfig();
        var lifecycleBaseUrl = connectionConfig.GetBaseUrl();
        var adapterFactory = options.AdapterFactory ?? DefaultAdapterFactory.Create();

        ISandboxes sandboxes;
        try
        {
            var lifecycleStack = adapterFactory.CreateLifecycleStack(new CreateLifecycleStackOptions
            {
                ConnectionConfig = connectionConfig,
                LifecycleBaseUrl = lifecycleBaseUrl
            });
            sandboxes = lifecycleStack.Sandboxes;
        }
        catch
        {
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
            Extensions = options.Extensions?.ToDictionary(kv => kv.Key, kv => (object)kv.Value)
        };

        string? sandboxId = null;
        try
        {
            var created = await sandboxes.CreateSandboxAsync(request, cancellationToken).ConfigureAwait(false);
            sandboxId = created.Id;

            var endpoint = await sandboxes.GetSandboxEndpointAsync(sandboxId, Constants.DefaultExecdPort, cancellationToken).ConfigureAwait(false);
            var protocol = connectionConfig.Protocol == ConnectionProtocol.Https ? "https" : "http";
            var execdBaseUrl = $"{protocol}://{endpoint.EndpointAddress}";

            var execdStack = adapterFactory.CreateExecdStack(new CreateExecdStackOptions
            {
                ConnectionConfig = connectionConfig,
                ExecdBaseUrl = execdBaseUrl
            });

            var sandbox = new Sandbox(
                sandboxId,
                connectionConfig,
                adapterFactory,
                lifecycleBaseUrl,
                execdBaseUrl,
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
        catch
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
            throw;
        }
    }

    /// <summary>
    /// Connects to an existing sandbox.
    /// </summary>
    /// <param name="options">The connection options.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The connected sandbox.</returns>
    public static async Task<Sandbox> ConnectAsync(
        SandboxConnectOptions options,
        CancellationToken cancellationToken = default)
    {
        var connectionConfig = options.ConnectionConfig ?? new ConnectionConfig();
        var lifecycleBaseUrl = connectionConfig.GetBaseUrl();
        var adapterFactory = options.AdapterFactory ?? DefaultAdapterFactory.Create();

        var lifecycleStack = adapterFactory.CreateLifecycleStack(new CreateLifecycleStackOptions
        {
            ConnectionConfig = connectionConfig,
            LifecycleBaseUrl = lifecycleBaseUrl
        });
        var sandboxes = lifecycleStack.Sandboxes;

        var endpoint = await sandboxes.GetSandboxEndpointAsync(options.SandboxId, Constants.DefaultExecdPort, cancellationToken).ConfigureAwait(false);
        var protocol = connectionConfig.Protocol == ConnectionProtocol.Https ? "https" : "http";
        var execdBaseUrl = $"{protocol}://{endpoint.EndpointAddress}";

        var execdStack = adapterFactory.CreateExecdStack(new CreateExecdStackOptions
        {
            ConnectionConfig = connectionConfig,
            ExecdBaseUrl = execdBaseUrl
        });

        var sandbox = new Sandbox(
            options.SandboxId,
            connectionConfig,
            adapterFactory,
            lifecycleBaseUrl,
            execdBaseUrl,
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

    /// <summary>
    /// Resumes a paused sandbox by ID.
    /// </summary>
    /// <param name="options">The connection options.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The resumed sandbox.</returns>
    public static async Task<Sandbox> ResumeAsync(
        SandboxConnectOptions options,
        CancellationToken cancellationToken = default)
    {
        var connectionConfig = options.ConnectionConfig ?? new ConnectionConfig();
        var lifecycleBaseUrl = connectionConfig.GetBaseUrl();
        var adapterFactory = options.AdapterFactory ?? DefaultAdapterFactory.Create();

        var lifecycleStack = adapterFactory.CreateLifecycleStack(new CreateLifecycleStackOptions
        {
            ConnectionConfig = connectionConfig,
            LifecycleBaseUrl = lifecycleBaseUrl
        });

        await lifecycleStack.Sandboxes.ResumeSandboxAsync(options.SandboxId, cancellationToken).ConfigureAwait(false);

        return await ConnectAsync(options, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Gets information about this sandbox.
    /// </summary>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The sandbox information.</returns>
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
        catch
        {
            return false;
        }
    }

    /// <summary>
    /// Gets the current resource metrics.
    /// </summary>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The sandbox metrics.</returns>
    public Task<SandboxMetrics> GetMetricsAsync(CancellationToken cancellationToken = default)
    {
        return Metrics.GetMetricsAsync(cancellationToken);
    }

    /// <summary>
    /// Pauses the sandbox.
    /// </summary>
    /// <param name="cancellationToken">Cancellation token.</param>
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
    public async Task<Sandbox> ResumeAsync(
        SandboxResumeOptions? options = null,
        CancellationToken cancellationToken = default)
    {
        await _sandboxes.ResumeSandboxAsync(Id, cancellationToken).ConfigureAwait(false);

        return await ConnectAsync(new SandboxConnectOptions
        {
            SandboxId = Id,
            ConnectionConfig = ConnectionConfig,
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
    public Task<Endpoint> GetEndpointAsync(int port, CancellationToken cancellationToken = default)
    {
        return _sandboxes.GetSandboxEndpointAsync(Id, port, cancellationToken);
    }

    /// <summary>
    /// Gets the endpoint URL for a port.
    /// </summary>
    /// <param name="port">The port number.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The endpoint URL.</returns>
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
    public async Task WaitUntilReadyAsync(
        WaitUntilReadyOptions options,
        CancellationToken cancellationToken = default)
    {
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
                    return;
                }
            }
            catch
            {
                // Ignore and retry
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
        // Note: HttpClient instances are managed by the ConnectionConfig
        // and may be shared, so we don't dispose them here
        return default;
    }
}

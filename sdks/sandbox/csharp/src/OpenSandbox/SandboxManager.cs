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
using OpenSandbox.Factory;
using OpenSandbox.Models;
using OpenSandbox.Services;

namespace OpenSandbox;

/// <summary>
/// Administrative interface for managing sandboxes.
/// </summary>
public sealed class SandboxManager : IAsyncDisposable
{
    private readonly ISandboxes _sandboxes;
    private readonly ConnectionConfig _connectionConfig;
    private bool _disposed;

    private SandboxManager(ISandboxes sandboxes, ConnectionConfig connectionConfig)
    {
        _sandboxes = sandboxes;
        _connectionConfig = connectionConfig;
    }

    /// <summary>
    /// Creates a new sandbox manager.
    /// </summary>
    /// <param name="options">Optional configuration options.</param>
    /// <returns>A new sandbox manager instance.</returns>
    public static SandboxManager Create(SandboxManagerOptions? options = null)
    {
        var connectionConfig = options?.ConnectionConfig ?? new ConnectionConfig();
        var lifecycleBaseUrl = connectionConfig.GetBaseUrl();
        var adapterFactory = options?.AdapterFactory ?? DefaultAdapterFactory.Create();

        var lifecycleStack = adapterFactory.CreateLifecycleStack(new CreateLifecycleStackOptions
        {
            ConnectionConfig = connectionConfig,
            LifecycleBaseUrl = lifecycleBaseUrl
        });

        return new SandboxManager(lifecycleStack.Sandboxes, connectionConfig);
    }

    /// <summary>
    /// Lists sandboxes with optional filtering.
    /// </summary>
    /// <param name="filter">Optional filter criteria.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The list of sandboxes.</returns>
    public Task<ListSandboxesResponse> ListSandboxInfosAsync(
        SandboxFilter? filter = null,
        CancellationToken cancellationToken = default)
    {
        return _sandboxes.ListSandboxesAsync(new ListSandboxesParams
        {
            States = filter?.States,
            Metadata = filter?.Metadata,
            Page = filter?.Page,
            PageSize = filter?.PageSize
        }, cancellationToken);
    }

    /// <summary>
    /// Gets information about a specific sandbox.
    /// </summary>
    /// <param name="sandboxId">The sandbox ID.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>The sandbox information.</returns>
    public Task<SandboxInfo> GetSandboxInfoAsync(
        string sandboxId,
        CancellationToken cancellationToken = default)
    {
        return _sandboxes.GetSandboxAsync(sandboxId, cancellationToken);
    }

    /// <summary>
    /// Terminates a sandbox.
    /// </summary>
    /// <param name="sandboxId">The sandbox ID.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    public Task KillSandboxAsync(
        string sandboxId,
        CancellationToken cancellationToken = default)
    {
        return _sandboxes.DeleteSandboxAsync(sandboxId, cancellationToken);
    }

    /// <summary>
    /// Pauses a sandbox.
    /// </summary>
    /// <param name="sandboxId">The sandbox ID.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    public Task PauseSandboxAsync(
        string sandboxId,
        CancellationToken cancellationToken = default)
    {
        return _sandboxes.PauseSandboxAsync(sandboxId, cancellationToken);
    }

    /// <summary>
    /// Resumes a paused sandbox.
    /// </summary>
    /// <param name="sandboxId">The sandbox ID.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    public Task ResumeSandboxAsync(
        string sandboxId,
        CancellationToken cancellationToken = default)
    {
        return _sandboxes.ResumeSandboxAsync(sandboxId, cancellationToken);
    }

    /// <summary>
    /// Renews the expiration time of a sandbox.
    /// </summary>
    /// <param name="sandboxId">The sandbox ID.</param>
    /// <param name="timeoutSeconds">The new timeout in seconds from now.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    public async Task RenewSandboxAsync(
        string sandboxId,
        int timeoutSeconds,
        CancellationToken cancellationToken = default)
    {
        var expiresAt = DateTime.UtcNow.AddSeconds(timeoutSeconds).ToString("O");
        await _sandboxes.RenewSandboxExpirationAsync(sandboxId, new RenewSandboxExpirationRequest
        {
            ExpiresAt = expiresAt
        }, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>
    /// Releases resources used by this manager.
    /// </summary>
    public ValueTask DisposeAsync()
    {
        if (_disposed)
        {
            return default;
        }

        _disposed = true;
        return default;
    }
}

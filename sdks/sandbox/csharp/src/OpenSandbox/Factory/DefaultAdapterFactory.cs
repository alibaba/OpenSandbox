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

using OpenSandbox.Adapters;
using OpenSandbox.Internal;

namespace OpenSandbox.Factory;

/// <summary>
/// Default implementation of the adapter factory.
/// </summary>
public sealed class DefaultAdapterFactory : IAdapterFactory
{
    /// <summary>
    /// Creates a new instance of the default adapter factory.
    /// </summary>
    /// <returns>A new adapter factory instance.</returns>
    public static IAdapterFactory Create() => new DefaultAdapterFactory();

    /// <inheritdoc />
    public LifecycleStack CreateLifecycleStack(CreateLifecycleStackOptions options)
    {
        var httpClient = options.ConnectionConfig.CreateHttpClient();
        var clientWrapper = new HttpClientWrapper(
            httpClient,
            options.LifecycleBaseUrl,
            options.ConnectionConfig.Headers);

        var sandboxes = new SandboxesAdapter(clientWrapper);

        return new LifecycleStack
        {
            Sandboxes = sandboxes
        };
    }

    /// <inheritdoc />
    public ExecdStack CreateExecdStack(CreateExecdStackOptions options)
    {
        var httpClient = options.ConnectionConfig.CreateHttpClient();
        var sseHttpClient = options.ConnectionConfig.CreateSseHttpClient();

        var clientWrapper = new HttpClientWrapper(
            httpClient,
            options.ExecdBaseUrl,
            options.ConnectionConfig.Headers);

        var health = new HealthAdapter(clientWrapper);
        var metrics = new MetricsAdapter(clientWrapper);
        var files = new FilesystemAdapter(
            clientWrapper,
            httpClient,
            options.ExecdBaseUrl,
            options.ConnectionConfig.Headers);
        var commands = new CommandsAdapter(
            clientWrapper,
            sseHttpClient,
            options.ExecdBaseUrl,
            options.ConnectionConfig.Headers);

        return new ExecdStack
        {
            Commands = commands,
            Files = files,
            Health = health,
            Metrics = metrics
        };
    }
}

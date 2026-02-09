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

namespace OpenSandbox.E2ETests;

/// <summary>
/// Shared fixture for E2E tests providing common configuration.
/// </summary>
public class E2ETestFixture : IAsyncLifetime
{
    /// <summary>
    /// Gets the default sandbox image for tests.
    /// </summary>
    public string DefaultImage { get; }

    /// <summary>
    /// Gets the connection configuration for tests.
    /// </summary>
    public ConnectionConfig ConnectionConfig { get; }

    /// <summary>
    /// Gets the default timeout in seconds for sandbox creation.
    /// </summary>
    public int DefaultTimeoutSeconds { get; } = 300;

    /// <summary>
    /// Gets the default ready timeout in seconds.
    /// </summary>
    public int DefaultReadyTimeoutSeconds { get; } = 60;

    public E2ETestFixture()
    {
        // Read configuration from environment variables
        DefaultImage = Environment.GetEnvironmentVariable("SANDBOX_IMAGE") ?? "python:3.11-slim";

        var domain = Environment.GetEnvironmentVariable("SANDBOX_DOMAIN") ?? "localhost:8080";
        var apiKey = Environment.GetEnvironmentVariable("SANDBOX_API_KEY");
        var protocolStr = Environment.GetEnvironmentVariable("SANDBOX_PROTOCOL") ?? "http";
        var protocol = protocolStr.Equals("https", StringComparison.OrdinalIgnoreCase)
            ? ConnectionProtocol.Https
            : ConnectionProtocol.Http;

        ConnectionConfig = new ConnectionConfig(new ConnectionConfigOptions
        {
            Domain = domain,
            Protocol = protocol,
            ApiKey = apiKey,
            RequestTimeoutSeconds = 120,
            Debug = Environment.GetEnvironmentVariable("SANDBOX_DEBUG") == "true"
        });
    }

    public Task InitializeAsync()
    {
        return Task.CompletedTask;
    }

    public Task DisposeAsync()
    {
        return Task.CompletedTask;
    }
}

/// <summary>
/// Collection definition for E2E tests.
/// </summary>
[CollectionDefinition("E2E Tests")]
public class E2ETestCollection : ICollectionFixture<E2ETestFixture>
{
}

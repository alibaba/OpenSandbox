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

using OpenSandbox.CodeInterpreter.Factory;
using OpenSandbox.CodeInterpreter.Services;
using OpenSandbox.Config;
using OpenSandbox.Core;
using OpenSandbox.Services;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace OpenSandbox.CodeInterpreter;

/// <summary>
/// Options for creating a code interpreter.
/// </summary>
public class CodeInterpreterCreateOptions
{
    /// <summary>
    /// Gets or sets the adapter factory. If not provided, a default factory is used.
    /// </summary>
    public ICodeInterpreterAdapterFactory? AdapterFactory { get; set; }

    /// <summary>
    /// Gets or sets diagnostics options such as logging.
    /// </summary>
    public SdkDiagnosticsOptions? Diagnostics { get; set; }
}

/// <summary>
/// Code interpreter facade for executing code in multiple languages.
/// </summary>
/// <remarks>
/// This class wraps an existing <see cref="Sandbox"/> and provides a high-level API for code execution.
/// Use <see cref="Codes"/> to create contexts and run code.
/// <see cref="Files"/>, <see cref="Commands"/>, and <see cref="Metrics"/> are exposed for convenience
/// and are the same instances as on the underlying <see cref="Sandbox"/>.
/// This type does not own the remote sandbox lifecycle. Call <see cref="Sandbox.KillAsync"/> when you want to terminate
/// the remote instance. Dispose the wrapped <see cref="Sandbox"/> to release local SDK resources.
/// </remarks>
public sealed class CodeInterpreter
{
    /// <summary>
    /// Gets the underlying sandbox instance.
    /// </summary>
    public Sandbox Sandbox { get; }

    /// <summary>
    /// Gets the codes service for code execution operations.
    /// </summary>
    public ICodes Codes { get; }

    /// <summary>
    /// Gets the sandbox ID.
    /// </summary>
    public string Id => Sandbox.Id;

    /// <summary>
    /// Gets the filesystem service.
    /// </summary>
    public ISandboxFiles Files => Sandbox.Files;

    /// <summary>
    /// Gets the command execution service.
    /// </summary>
    public IExecdCommands Commands => Sandbox.Commands;

    /// <summary>
    /// Gets the metrics service.
    /// </summary>
    public IExecdMetrics Metrics => Sandbox.Metrics;

    private readonly ILogger _logger;

    private CodeInterpreter(Sandbox sandbox, ICodes codes, ILogger logger)
    {
        Sandbox = sandbox ?? throw new ArgumentNullException(nameof(sandbox));
        Codes = codes ?? throw new ArgumentNullException(nameof(codes));
        _logger = logger ?? throw new ArgumentNullException(nameof(logger));
        _logger.LogDebug("Code interpreter initialized for sandbox: {SandboxId}", sandbox.Id);
    }

    /// <summary>
    /// Creates a new code interpreter from an existing sandbox.
    /// </summary>
    /// <param name="sandbox">The sandbox to wrap.</param>
    /// <param name="options">Optional creation options.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>A new code interpreter instance.</returns>
    /// <exception cref="InvalidArgumentException">Thrown when <paramref name="sandbox"/> is null.</exception>
    /// <exception cref="SandboxException">Thrown when endpoint discovery or adapter initialization fails.</exception>
    public static async Task<CodeInterpreter> CreateAsync(
        Sandbox sandbox,
        CodeInterpreterCreateOptions? options = null,
        CancellationToken cancellationToken = default)
    {
        if (sandbox == null)
        {
            throw new InvalidArgumentException("sandbox cannot be null");
        }

        var loggerFactory = options?.Diagnostics?.LoggerFactory ?? sandbox.SharedLoggerFactory ?? NullLoggerFactory.Instance;
        var logger = loggerFactory.CreateLogger("OpenSandbox.CodeInterpreter.CodeInterpreter");
        var endpoint = await sandbox.GetEndpointAsync(Constants.DefaultExecdPort, cancellationToken).ConfigureAwait(false);
        logger.LogInformation("Creating code interpreter for sandbox: {SandboxId}", sandbox.Id);
        var protocol = sandbox.ConnectionConfig.Protocol == ConnectionProtocol.Https ? "https" : "http";
        var execdBaseUrl = $"{protocol}://{endpoint.EndpointAddress}";
        var execdHeaders = MergeHeaders(sandbox.ConnectionConfig.Headers, endpoint.Headers);
        var adapterFactory = options?.AdapterFactory ?? DefaultCodeInterpreterAdapterFactory.Create();

        var codes = adapterFactory.CreateCodes(new CreateCodesStackOptions
        {
            ConnectionConfig = sandbox.ConnectionConfig,
            ExecdBaseUrl = execdBaseUrl,
            ExecdHeaders = execdHeaders,
            HttpClientProvider = sandbox.SharedHttpClientProvider,
            LoggerFactory = loggerFactory
        });

        return new CodeInterpreter(sandbox, codes, logger);
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

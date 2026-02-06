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
using OpenSandbox.Core;
using OpenSandbox.Services;

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
}

/// <summary>
/// Code interpreter facade for executing code in multiple languages.
/// </summary>
/// <remarks>
/// This class wraps an existing <see cref="Sandbox"/> and provides a high-level API for code execution.
/// Use <see cref="Codes"/> to create contexts and run code.
/// <see cref="Files"/>, <see cref="Commands"/>, and <see cref="Metrics"/> are exposed for convenience
/// and are the same instances as on the underlying <see cref="Sandbox"/>.
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

    private CodeInterpreter(Sandbox sandbox, ICodes codes)
    {
        Sandbox = sandbox ?? throw new ArgumentNullException(nameof(sandbox));
        Codes = codes ?? throw new ArgumentNullException(nameof(codes));
    }

    /// <summary>
    /// Creates a new code interpreter from an existing sandbox.
    /// </summary>
    /// <param name="sandbox">The sandbox to wrap.</param>
    /// <param name="options">Optional creation options.</param>
    /// <param name="cancellationToken">Cancellation token.</param>
    /// <returns>A new code interpreter instance.</returns>
    public static async Task<CodeInterpreter> CreateAsync(
        Sandbox sandbox,
        CodeInterpreterCreateOptions? options = null,
        CancellationToken cancellationToken = default)
    {
        if (sandbox == null)
        {
            throw new ArgumentNullException(nameof(sandbox));
        }

        var execdBaseUrl = await sandbox.GetEndpointUrlAsync(Constants.DefaultExecdPort, cancellationToken).ConfigureAwait(false);
        var adapterFactory = options?.AdapterFactory ?? DefaultCodeInterpreterAdapterFactory.Create();

        var codes = adapterFactory.CreateCodes(new CreateCodesStackOptions
        {
            Sandbox = sandbox,
            ExecdBaseUrl = execdBaseUrl
        });

        return new CodeInterpreter(sandbox, codes);
    }
}

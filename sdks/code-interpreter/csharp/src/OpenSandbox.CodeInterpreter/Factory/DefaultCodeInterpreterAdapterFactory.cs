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

using OpenSandbox.CodeInterpreter.Adapters;
using OpenSandbox.CodeInterpreter.Services;
using OpenSandbox.Internal;

namespace OpenSandbox.CodeInterpreter.Factory;

/// <summary>
/// Default implementation of the code interpreter adapter factory.
/// </summary>
public class DefaultCodeInterpreterAdapterFactory : ICodeInterpreterAdapterFactory
{
    /// <summary>
    /// Creates a new instance of the default adapter factory.
    /// </summary>
    /// <returns>A new factory instance.</returns>
    public static DefaultCodeInterpreterAdapterFactory Create() => new();

    /// <inheritdoc />
    public ICodes CreateCodes(CreateCodesStackOptions options)
    {
        if (options == null)
        {
            throw new ArgumentNullException(nameof(options));
        }

        if (options.Sandbox == null)
        {
            throw new ArgumentNullException(nameof(options.Sandbox));
        }

        if (string.IsNullOrWhiteSpace(options.ExecdBaseUrl))
        {
            throw new ArgumentNullException(nameof(options.ExecdBaseUrl));
        }

        var connectionConfig = options.Sandbox.ConnectionConfig;
        var httpClient = connectionConfig.CreateHttpClient();
        var sseHttpClient = connectionConfig.CreateSseHttpClient();

        var client = new HttpClientWrapper(
            httpClient,
            options.ExecdBaseUrl,
            connectionConfig.Headers);

        return new CodesAdapter(
            client,
            sseHttpClient,
            options.ExecdBaseUrl,
            connectionConfig.Headers);
    }
}

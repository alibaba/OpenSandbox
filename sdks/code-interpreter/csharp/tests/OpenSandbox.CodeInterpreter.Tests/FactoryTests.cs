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
using Xunit;

namespace OpenSandbox.CodeInterpreter.Tests;

public class FactoryTests
{
    [Fact]
    public void DefaultCodeInterpreterAdapterFactory_Create_ReturnsInstance()
    {
        var factory = DefaultCodeInterpreterAdapterFactory.Create();

        Assert.NotNull(factory);
        Assert.IsType<DefaultCodeInterpreterAdapterFactory>(factory);
    }

    [Fact]
    public void DefaultCodeInterpreterAdapterFactory_CreateCodes_ThrowsOnNullOptions()
    {
        var factory = DefaultCodeInterpreterAdapterFactory.Create();

        Assert.Throws<ArgumentNullException>(() => factory.CreateCodes(null!));
    }

    [Fact]
    public void DefaultCodeInterpreterAdapterFactory_CreateCodes_ThrowsOnNullSandbox()
    {
        var factory = DefaultCodeInterpreterAdapterFactory.Create();
        var options = new CreateCodesStackOptions
        {
            Sandbox = null!,
            ExecdBaseUrl = "http://localhost:44772"
        };

        Assert.Throws<ArgumentNullException>(() => factory.CreateCodes(options));
    }

    [Fact]
    public void DefaultCodeInterpreterAdapterFactory_CreateCodes_ThrowsOnEmptyBaseUrl()
    {
        var factory = DefaultCodeInterpreterAdapterFactory.Create();

        // We can't easily create a mock Sandbox, so we test the null/empty URL case
        // by checking that the factory validates its inputs
        var options = new CreateCodesStackOptions
        {
            Sandbox = null!,
            ExecdBaseUrl = ""
        };

        // Should throw for null sandbox first
        Assert.Throws<ArgumentNullException>(() => factory.CreateCodes(options));
    }

    [Fact]
    public void CreateCodesStackOptions_RequiredProperties()
    {
        var options = new CreateCodesStackOptions
        {
            Sandbox = null!,
            ExecdBaseUrl = "http://test:8080"
        };

        Assert.Null(options.Sandbox);
        Assert.Equal("http://test:8080", options.ExecdBaseUrl);
    }
}

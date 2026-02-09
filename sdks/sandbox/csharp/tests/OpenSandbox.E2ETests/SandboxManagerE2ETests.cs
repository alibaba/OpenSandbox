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

using OpenSandbox;
using OpenSandbox.Models;
using Xunit;

namespace OpenSandbox.E2ETests;

[Collection("E2E Tests")]
public class SandboxManagerE2ETests : IAsyncLifetime
{
    private readonly E2ETestFixture _fixture;
    private SandboxManager? _manager;
    private Sandbox? _s1;
    private Sandbox? _s2;
    private Sandbox? _s3;
    private string? _tag;

    public SandboxManagerE2ETests(E2ETestFixture fixture)
    {
        _fixture = fixture;
    }

    public async Task InitializeAsync()
    {
        _manager = SandboxManager.Create(new SandboxManagerOptions
        {
            ConnectionConfig = _fixture.ConnectionConfig
        });

        _tag = $"e2e-sandbox-manager-{Guid.NewGuid():N}"[..18];

        _s1 = await CreateSandboxAsync(new Dictionary<string, string>
        {
            ["tag"] = _tag,
            ["team"] = "t1",
            ["env"] = "prod"
        });

        _s2 = await CreateSandboxAsync(new Dictionary<string, string>
        {
            ["tag"] = _tag,
            ["team"] = "t1",
            ["env"] = "dev"
        });

        _s3 = await CreateSandboxAsync(new Dictionary<string, string>
        {
            ["tag"] = _tag,
            ["env"] = "prod"
        });

        await _manager.PauseSandboxAsync(_s3.Id);
        await WaitForStateAsync(_s3.Id, SandboxStates.Paused, TimeSpan.FromMinutes(3));
    }

    public async Task DisposeAsync()
    {
        foreach (var sandbox in new[] { _s1, _s2, _s3 })
        {
            if (sandbox == null)
            {
                continue;
            }

            try
            {
                await sandbox.KillAsync();
            }
            catch
            {
            }

            await sandbox.DisposeAsync();
        }

        if (_manager is not null)
        {
            await _manager.DisposeAsync();
        }
    }

    [Fact(Timeout = 10 * 60 * 1000)]
    public async Task ListSandboxInfos_StatesFilter_IsOrLogic()
    {
        Assert.NotNull(_manager);
        Assert.NotNull(_tag);
        Assert.NotNull(_s1);
        Assert.NotNull(_s2);
        Assert.NotNull(_s3);

        var result = await _manager.ListSandboxInfosAsync(new SandboxFilter
        {
            States = new[] { SandboxStates.Running, SandboxStates.Paused },
            Metadata = new Dictionary<string, string> { ["tag"] = _tag },
            PageSize = 50
        });

        var ids = result.Items.Select(info => info.Id).ToHashSet();
        Assert.True(ids.Contains(_s1.Id));
        Assert.True(ids.Contains(_s2.Id));
        Assert.True(ids.Contains(_s3.Id));

        var pausedOnly = await _manager.ListSandboxInfosAsync(new SandboxFilter
        {
            States = new[] { SandboxStates.Paused },
            Metadata = new Dictionary<string, string> { ["tag"] = _tag },
            PageSize = 50
        });

        var pausedIds = pausedOnly.Items.Select(info => info.Id).ToHashSet();
        Assert.Contains(_s3.Id, pausedIds);
        Assert.DoesNotContain(_s1.Id, pausedIds);
        Assert.DoesNotContain(_s2.Id, pausedIds);
    }

    [Fact(Timeout = 10 * 60 * 1000)]
    public async Task ListSandboxInfos_MetadataFilter_IsAndLogic()
    {
        Assert.NotNull(_manager);
        Assert.NotNull(_tag);
        Assert.NotNull(_s1);
        Assert.NotNull(_s2);
        Assert.NotNull(_s3);

        var tagAndTeam = await _manager.ListSandboxInfosAsync(new SandboxFilter
        {
            Metadata = new Dictionary<string, string> { ["tag"] = _tag, ["team"] = "t1" },
            PageSize = 50
        });

        var tagAndTeamIds = tagAndTeam.Items.Select(info => info.Id).ToHashSet();
        Assert.Contains(_s1.Id, tagAndTeamIds);
        Assert.Contains(_s2.Id, tagAndTeamIds);
        Assert.DoesNotContain(_s3.Id, tagAndTeamIds);

        var tagTeamEnv = await _manager.ListSandboxInfosAsync(new SandboxFilter
        {
            Metadata = new Dictionary<string, string>
            {
                ["tag"] = _tag,
                ["team"] = "t1",
                ["env"] = "prod"
            },
            PageSize = 50
        });

        var tagTeamEnvIds = tagTeamEnv.Items.Select(info => info.Id).ToHashSet();
        Assert.Contains(_s1.Id, tagTeamEnvIds);
        Assert.DoesNotContain(_s2.Id, tagTeamEnvIds);
        Assert.DoesNotContain(_s3.Id, tagTeamEnvIds);

        var tagEnv = await _manager.ListSandboxInfosAsync(new SandboxFilter
        {
            Metadata = new Dictionary<string, string> { ["tag"] = _tag, ["env"] = "prod" },
            PageSize = 50
        });

        var tagEnvIds = tagEnv.Items.Select(info => info.Id).ToHashSet();
        Assert.Contains(_s1.Id, tagEnvIds);
        Assert.Contains(_s3.Id, tagEnvIds);
        Assert.DoesNotContain(_s2.Id, tagEnvIds);

        var noneMatch = await _manager.ListSandboxInfosAsync(new SandboxFilter
        {
            Metadata = new Dictionary<string, string> { ["tag"] = _tag, ["team"] = "t2" },
            PageSize = 50
        });

        var noneIds = noneMatch.Items.Select(info => info.Id).ToHashSet();
        Assert.DoesNotContain(_s1.Id, noneIds);
        Assert.DoesNotContain(_s2.Id, noneIds);
        Assert.DoesNotContain(_s3.Id, noneIds);
    }

    private async Task<Sandbox> CreateSandboxAsync(IReadOnlyDictionary<string, string> metadata)
    {
        return await Sandbox.CreateAsync(new SandboxCreateOptions
        {
            ConnectionConfig = _fixture.ConnectionConfig,
            Image = _fixture.DefaultImage,
            TimeoutSeconds = _fixture.DefaultTimeoutSeconds,
            ReadyTimeoutSeconds = _fixture.DefaultReadyTimeoutSeconds,
            Metadata = metadata,
            Env = new Dictionary<string, string> { ["E2E_TEST"] = "true" },
            HealthCheckPollingInterval = 500
        });
    }

    private async Task WaitForStateAsync(string sandboxId, string expectedState, TimeSpan timeout)
    {
        var deadline = DateTime.UtcNow + timeout;
        while (true)
        {
            var info = await _manager!.GetSandboxInfoAsync(sandboxId);
            if (info.Status.State == expectedState)
            {
                return;
            }

            if (DateTime.UtcNow > deadline)
            {
                throw new TimeoutException(
                    $"Timed out waiting for state={expectedState}, last_state={info.Status.State}");
            }

            await Task.Delay(1000);
        }
    }
}

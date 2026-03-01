// Copyright 2025 Alibaba Group Holding Ltd.
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

package gvisor

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/test/e2e_runtime"
)

const (
	// KindCluster is the name of the Kind cluster for gVisor tests
	KindCluster = "gvisor-test"

	// RuntimeClassName is the name of the RuntimeClass for gVisor
	RuntimeClassName = "gvisor"
)

// TestGVisorRuntimeClass runs the gVisor RuntimeClass end-to-end tests.
// These tests validate gVisor functionality with the Kind cluster
// configured specifically for gVisor (runsc) runtime.
func TestGVisorRuntimeClass(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting gVisor RuntimeClass E2E test suite\n")
	RunSpecs(t, "gVisor runtimeclass suite")
}

var _ = BeforeSuite(func() {
	dockerBuildArgs := os.Getenv("DOCKER_BUILD_ARGS")

	By("building task-executor image")
	makeArgs := []string{"docker-build-task-executor", fmt.Sprintf("TASK_EXECUTOR_IMG=%s", e2e_runtime.TaskExecutorImage)}
	if dockerBuildArgs != "" {
		makeArgs = append(makeArgs, fmt.Sprintf("DOCKER_BUILD_ARGS=%s", dockerBuildArgs))
	}
	cmd := exec.Command("make", makeArgs...)
	cmd.Dir = "../../.." // 从 test/e2e_runtime/gvisor 回到项目根目录
	output, err := cmd.CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build task-executor image: %s", string(output))

	By("loading task-executor image on Kind")
	// 直接使用 kind 命令加载镜像，避免 utils.GetProjectDir() 路径问题
	cmd = exec.Command("kind", "load", "docker-image", "--name", KindCluster, e2e_runtime.TaskExecutorImage)
	cmd.Dir = "../../.." // 从 test/e2e_runtime/gvisor 回到项目根目录
	output, err = cmd.CombinedOutput()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load task-executor image into Kind: %s", string(output))
})

var _ = AfterSuite(func() {
})

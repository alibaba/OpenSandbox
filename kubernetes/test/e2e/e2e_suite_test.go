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

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/test/utils"
)

var (
	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/sandbox-k8s:v0.0.1"

	// taskExecutorImage is the name of the task-executor image
	taskExecutorImage = "example.com/task-executor:v0.0.1"

	// sandboxImage is a lightweight image used for sandbox containers in tests
	// Using task-executor image instead of ubuntu:latest to avoid download issues in certain network environments
	sandboxImage = taskExecutorImage
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purposed to be used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting sandbox-k8s integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	dockerBuildArgs := os.Getenv("DOCKER_BUILD_ARGS")

	By("building the manager(Operator) image")
	makeArgs := []string{"docker-build", fmt.Sprintf("IMG=%s", projectImage)}
	if dockerBuildArgs != "" {
		makeArgs = append(makeArgs, fmt.Sprintf("DOCKER_BUILD_ARGS=%s", dockerBuildArgs))
	}
	cmd := exec.Command("make", makeArgs...)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	By("building the task-executor image")
	makeArgs = []string{"docker-build-task-executor", fmt.Sprintf("TASK_EXECUTOR_IMG=%s", taskExecutorImage)}
	if dockerBuildArgs != "" {
		makeArgs = append(makeArgs, fmt.Sprintf("DOCKER_BUILD_ARGS=%s", dockerBuildArgs))
	}
	cmd = exec.Command("make", makeArgs...)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the task-executor image")

	// If you want to change the e2e test vendor from Kind, ensure the image is
	// built and available before running the tests. Also, remove the following block.
	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	By("loading the task-executor image on Kind")
	err = utils.LoadImageToKindClusterWithName(taskExecutorImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the task-executor image into Kind")
})

var _ = AfterSuite(func() {
})

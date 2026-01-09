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

package e2e_task

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/api/v1alpha1"
	api "github.com/alibaba/OpenSandbox/sandbox-k8s/pkg/task-executor"
)

const (
	ImageName         = "task-executor-e2e"
	TargetContainer   = "task-e2e-target"
	ExecutorContainer = "task-e2e-executor"
	VolumeName        = "task-e2e-vol"
	HostPort          = "5758"
)

var _ = Describe("Task Executor E2E", Ordered, func() {
	var client *api.Client

	BeforeAll(func() {
		// Check docker
		_, err := exec.LookPath("docker")
		Expect(err).NotTo(HaveOccurred(), "Docker not found, skipping E2E test")

		By("Building image")
		cmd := exec.Command("docker", "build",
			"--build-arg", "PACKAGE=cmd/task-executor/main.go",
			"-t", ImageName, "-f", "../../Dockerfile", "../../")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		Expect(cmd.Run()).To(Succeed())

		By("Cleaning up previous runs")
		exec.Command("docker", "rm", "-f", TargetContainer, ExecutorContainer).Run()
		exec.Command("docker", "volume", "rm", VolumeName).Run()

		By("Creating shared volume")
		Expect(exec.Command("docker", "volume", "create", VolumeName).Run()).To(Succeed())

		By("Starting target container")
		targetCmd := exec.Command("docker", "run", "-d", "--name", TargetContainer,
			"-v", fmt.Sprintf("%s:/tmp/tasks", VolumeName),
			"-e", "SANDBOX_MAIN_CONTAINER=main",
			"-e", "TARGET_VAR=hello-from-target",
			"golang:1.24", "sleep", "infinity")
		targetCmd.Stdout = os.Stdout
		targetCmd.Stderr = os.Stderr
		Expect(targetCmd.Run()).To(Succeed())

		By("Starting executor container in Sidecar Mode")
		execCmd := exec.Command("docker", "run", "-d", "--name", ExecutorContainer,
			"-v", fmt.Sprintf("%s:/tmp/tasks", VolumeName),
			"--privileged",
			"-u", "0",
			"--pid=container:"+TargetContainer,
			"-p", HostPort+":5758",
			ImageName,
			"-enable-sidecar-mode=true",
			"-main-container-name=main",
			"-data-dir=/tmp/tasks")
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
		Expect(execCmd.Run()).To(Succeed())

		By("Waiting for executor to be ready")
		client = api.NewClient(fmt.Sprintf("http://127.0.0.1:%s", HostPort))
		Eventually(func() error {
			_, err := client.Get(context.Background())
			return err
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed(), "Executor failed to become ready")
	})

	AfterAll(func() {
		By("Cleaning up containers")
		if CurrentSpecReport().Failed() {
			By("Dumping logs")
			out, _ := exec.Command("docker", "logs", ExecutorContainer).CombinedOutput()
			fmt.Printf("Executor Logs:\n%s\n", string(out))
		}
		exec.Command("docker", "rm", "-f", TargetContainer, ExecutorContainer).Run()
		exec.Command("docker", "volume", "rm", VolumeName).Run()
	})

	Context("When creating a short-lived task", func() {
		taskName := "e2e-test-1"

		It("should run and succeed", func() {
			By("Creating task")
			task := &api.Task{
				Name: taskName,
				Spec: v1alpha1.TaskSpec{
					Process: &v1alpha1.ProcessTask{
						Command: []string{"sleep", "2"},
					},
				},
			}
			_, err := client.Set(context.Background(), task)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for task to succeed")
			Eventually(func(g Gomega) {
				got, err := client.Get(context.Background())
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(got).NotTo(BeNil())
				g.Expect(got.Name).To(Equal(taskName))

				// Verify state
				if got.Status.State.Terminated != nil {
					g.Expect(got.Status.State.Terminated.ExitCode).To(BeZero())
					g.Expect(got.Status.State.Terminated.Reason).To(Equal("Succeeded"))
				} else {
					// Fail if not terminated yet (so Eventually retries)
					g.Expect(got.Status.State.Terminated).NotTo(BeNil(), "Task status: %v", got.Status.State)
				}
			}, 10*time.Second, 1*time.Second).Should(Succeed())
		})

		It("should be deletable", func() {
			By("Deleting task")
			_, err := client.Set(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying deletion")
			Eventually(func() *api.Task {
				got, _ := client.Get(context.Background())
				return got
			}, 5*time.Second, 500*time.Millisecond).Should(BeNil())
		})
	})

	Context("When creating a task checking environment variables", func() {
		taskName := "e2e-env-test"

		It("should inherit environment variables from target container", func() {
			By("Creating task running 'env'")
			task := &api.Task{
				Name: taskName,
				Spec: v1alpha1.TaskSpec{
					Process: &v1alpha1.ProcessTask{
						Command: []string{"env"},
					},
				},
			}
			_, err := client.Set(context.Background(), task)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for task to succeed")
			Eventually(func(g Gomega) {
				got, err := client.Get(context.Background())
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(got).NotTo(BeNil())
				g.Expect(got.Name).To(Equal(taskName))
				g.Expect(got.Status.State.Terminated).NotTo(BeNil())
				g.Expect(got.Status.State.Terminated.ExitCode).To(BeZero())
			}, 10*time.Second, 1*time.Second).Should(Succeed())

			By("Verifying stdout contains target container env")
			// Read stdout.log from the executor container (which shares the volume)
			out, err := exec.Command("docker", "exec", ExecutorContainer, "cat", fmt.Sprintf("/tmp/tasks/%s/stdout.log", taskName)).CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), "Failed to read stdout.log: %s", string(out))

			outputStr := string(out)
			Expect(outputStr).To(ContainSubstring("TARGET_VAR=hello-from-target"), "Task environment should inherit from target container")
		})

		It("should be deletable", func() {
			By("Deleting task")
			_, err := client.Set(context.Background(), nil)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying deletion")
			Eventually(func() *api.Task {
				got, _ := client.Get(context.Background())
				return got
			}, 5*time.Second, 500*time.Millisecond).Should(BeNil())
		})
	})
})

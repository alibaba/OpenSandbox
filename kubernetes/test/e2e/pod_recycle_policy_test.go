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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/test/utils"
)

// Pod Recycle Policy E2E Tests
// Tests cover: Delete policy, Restart policy (success and failure paths), batch operations

var _ = Describe("Pod Recycle Policy", Ordered, func() {
	const testNamespace = "default"

	BeforeAll(func() {
		By("waiting for controller to be ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
				"-n", namespace, "-o", "jsonpath={.items[0].status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Running"))
		}, 2*time.Minute).Should(Succeed())
	})

	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	Context("Delete Policy", func() {
		It("should delete pod when BatchSandbox is deleted with Delete policy", func() {
			const poolName = "test-pool-delete-policy"
			const batchSandboxName = "test-bs-delete-policy"

			// Clean up any existing resources from previous test runs
			By("cleaning up any existing resources")
			cmd := exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			By("creating a Pool with Delete policy")
			poolYAML, err := renderTemplate("testdata/pool-with-recycle-policy.yaml", map[string]interface{}{
				"PoolName":          poolName,
				"Namespace":         testNamespace,
				"PodRecyclePolicy":  "Delete",
				"TaskExecutorImage": utils.TaskExecutorImage,
				"BufferMax":         2,
				"BufferMin":         1,
				"PoolMax":           2,
				"PoolMin":           2,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-delete-policy.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd = exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).To(Equal("2"))
			}, 2*time.Minute).Should(Succeed())

			By("creating a BatchSandbox")
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         1,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-delete-policy.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for BatchSandbox to allocate pod and recording pod name")
			var allocatedPodName string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				allocStatusJSON, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(allocStatusJSON).NotTo(BeEmpty())

				var allocStatus struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(allocStatus.Pods)).To(Equal(1))
				allocatedPodName = allocStatus.Pods[0]
			}, 2*time.Minute).Should(Succeed())

			By("deleting the BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the pod is deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", allocatedPodName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "Pod should be deleted")
				g.Expect(err.Error()).To(ContainSubstring("not found"))
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up the Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})
	})

	Context("Restart Policy - Success", func() {
		It("should restart and reuse pod when BatchSandbox is deleted with Restart policy", func() {
			const poolName = "test-pool-restart-success"
			const batchSandboxName1 = "test-bs-restart-1"
			const batchSandboxName2 = "test-bs-restart-2"

			// Clean up any existing resources from previous test runs
			By("cleaning up any existing resources")
			cmd := exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName1, batchSandboxName2, "-n", testNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			By("creating a Pool with Restart policy")
			poolYAML, err := renderTemplate("testdata/pool-with-recycle-policy.yaml", map[string]interface{}{
				"PoolName":          poolName,
				"Namespace":         testNamespace,
				"PodRecyclePolicy":  "Restart",
				"TaskExecutorImage": utils.TaskExecutorImage,
				"BufferMax":         1,
				"BufferMin":         1,
				"PoolMax":           2,
				"PoolMin":           2,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-restart-success.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd = exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to be ready with all pods available")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).To(Equal("2"))

				cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				availableStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(availableStr).To(Equal("2"), "All pods should be available")
			}, 2*time.Minute).Should(Succeed())

			By("creating first BatchSandbox with replicas=2 to use all pods")
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName1,
				"Namespace":        testNamespace,
				"Replicas":         2,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-restart-1.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for BatchSandbox to allocate pods and recording pod names")
			var allocatedPodNames []string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName1, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				allocStatusJSON, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(allocStatusJSON).NotTo(BeEmpty())

				var allocStatus struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(allocStatus.Pods)).To(Equal(2))
				allocatedPodNames = allocStatus.Pods
			}, 2*time.Minute).Should(Succeed())

			By("deleting the first BatchSandbox to trigger restart")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName1, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pods to restart and return to pool")
			Eventually(func(g Gomega) {
				// Check pool restarting count goes to 0
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.restarting}")
				restartingStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				if restartingStr == "" {
					restartingStr = "0"
				}
				g.Expect(restartingStr).To(Equal("0"), "Restarting count should be 0")

				// Check pool available count
				cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				availableStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(availableStr).To(Equal("2"), "Both pods should be available")

				// Verify both pods still exist and are Running
				for _, podName := range allocatedPodNames {
					cmd = exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
						"-o", "jsonpath={.status.phase}")
					phase, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(phase).To(Equal("Running"))
				}
			}, 2*time.Minute).Should(Succeed())

			By("creating second BatchSandbox to reuse the pods")
			bsYAML2, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName2,
				"Namespace":        testNamespace,
				"Replicas":         2,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile2 := filepath.Join("/tmp", "test-bs-restart-2.yaml")
			err = os.WriteFile(bsFile2, []byte(bsYAML2), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile2)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile2)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the same pods are allocated to second BatchSandbox")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName2, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				allocStatusJSON, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(allocStatusJSON).NotTo(BeEmpty())

				var allocStatus struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(allocStatus.Pods)).To(Equal(2), "Should have 2 pods allocated")

				// Verify the same pods are reused
				for _, originalPod := range allocatedPodNames {
					found := false
					for _, newPod := range allocStatus.Pods {
						if newPod == originalPod {
							found = true
							break
						}
					}
					g.Expect(found).To(BeTrue(), "Pod %s should be reused", originalPod)
				}
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName2, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})
	})

	Context("Restart Policy - Failure", func() {
		It("should delete pod when restart times out", func() {
			const poolName = "test-pool-restart-timeout"
			const batchSandboxName = "test-bs-timeout"

			// Clean up any existing resources from previous test runs
			By("cleaning up any existing resources")
			cmd := exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)

			By("creating a Pool with Restart policy (sleep infinity won't respond to SIGTERM)")
			poolYAML, err := renderTemplate("testdata/pool-with-recycle-policy.yaml", map[string]interface{}{
				"PoolName":          poolName,
				"Namespace":         testNamespace,
				"PodRecyclePolicy":  "Restart",
				"TaskExecutorImage": utils.TaskExecutorImage,
				"TimeoutTest":       true, // Use sleep infinity for timeout testing
				"BufferMax":         2,
				"BufferMin":         1,
				"PoolMax":           2,
				"PoolMin":           2,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-timeout.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd = exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to be ready with available pods")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				availableStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(availableStr).To(Equal("2"), "Should have 2 available pods")
			}, 2*time.Minute).Should(Succeed())

			By("creating a BatchSandbox")
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         1,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-timeout.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for BatchSandbox to allocate pod")
			var allocatedPodName string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				allocStatusJSON, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(allocStatusJSON).NotTo(BeEmpty())

				var allocStatus struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(allocStatus.Pods)).To(Equal(1))
				allocatedPodName = allocStatus.Pods[0]
			}, 2*time.Minute).Should(Succeed())

			By("deleting the BatchSandbox to trigger restart")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the pod is deleted after timeout (sleep infinity won't respond to SIGTERM)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", allocatedPodName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "Pod should be deleted after timeout")
				g.Expect(err.Error()).To(ContainSubstring("not found"))
			}, 4*time.Minute).Should(Succeed()) // restartTimeout (90s) + buffer

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})
	})

	Context("Batch Operations", func() {
		It("should handle multiple BatchSandbox deletions with Restart policy", func() {
			const poolName = "test-pool-restart-batch"
			const numBatchSandboxes = 4
			const replicasPerSandbox = 2

			By("creating a Pool with Restart policy")
			poolYAML, err := renderTemplate("testdata/pool-with-recycle-policy.yaml", map[string]interface{}{
				"PoolName":          poolName,
				"Namespace":         testNamespace,
				"PodRecyclePolicy":  "Restart",
				"TaskExecutorImage": utils.TaskExecutorImage,
				"BufferMax":         4,
				"BufferMin":         2,
				"PoolMax":           10,
				"PoolMin":           10,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-restart-batch.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to be ready with 10 pods")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).To(Equal("10"))
			}, 3*time.Minute).Should(Succeed())

			By("creating multiple BatchSandboxes and recording allocated pods")
			batchSandboxNames := make([]string, numBatchSandboxes)
			allocatedPodsPerSandbox := make(map[string][]string)

			for i := 0; i < numBatchSandboxes; i++ {
				batchSandboxName := fmt.Sprintf("test-bs-batch-%d", i)
				batchSandboxNames[i] = batchSandboxName

				bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
					"BatchSandboxName": batchSandboxName,
					"Namespace":        testNamespace,
					"Replicas":         replicasPerSandbox,
					"PoolName":         poolName,
				})
				Expect(err).NotTo(HaveOccurred())

				bsFile := filepath.Join("/tmp", fmt.Sprintf("test-bs-batch-%d.yaml", i))
				err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
				Expect(err).NotTo(HaveOccurred())
				defer os.Remove(bsFile)

				cmd := exec.Command("kubectl", "apply", "-f", bsFile)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
			}

			By("waiting for all BatchSandboxes to allocate pods")
			for i := 0; i < numBatchSandboxes; i++ {
				batchSandboxName := batchSandboxNames[i]
				Eventually(func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
						"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
					allocStatusJSON, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(allocStatusJSON).NotTo(BeEmpty())

					var allocStatus struct {
						Pods []string `json:"pods"`
					}
					err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(len(allocStatus.Pods)).To(Equal(replicasPerSandbox))
					allocatedPodsPerSandbox[batchSandboxName] = allocStatus.Pods
				}, 2*time.Minute).Should(Succeed())
			}

			By("deleting BatchSandboxes serially and verifying pods restart")
			for i := 0; i < numBatchSandboxes; i++ {
				batchSandboxName := batchSandboxNames[i]

				By(fmt.Sprintf("deleting BatchSandbox %s", batchSandboxName))
				cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("waiting for pods from %s to restart and return to pool", batchSandboxName))
				Eventually(func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
						"-o", "jsonpath={.status.restarting}")
					restartingStr, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					if restartingStr == "" {
						restartingStr = "0"
					}
					g.Expect(restartingStr).To(Equal("0"), "Restarting count should be 0")
				}, 2*time.Minute).Should(Succeed())
			}

			By("verifying all pods are still available in the pool")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).To(Equal("10"))

				cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				availableStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(availableStr).To(Equal("10"), "All pods should be available")
			}, 2*time.Minute).Should(Succeed())

			By("creating new BatchSandboxes to verify pod reuse")
			for i := 0; i < numBatchSandboxes; i++ {
				newBatchSandboxName := fmt.Sprintf("test-bs-batch-new-%d", i)

				bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
					"BatchSandboxName": newBatchSandboxName,
					"Namespace":        testNamespace,
					"Replicas":         replicasPerSandbox,
					"PoolName":         poolName,
				})
				Expect(err).NotTo(HaveOccurred())

				bsFile := filepath.Join("/tmp", fmt.Sprintf("test-bs-batch-new-%d.yaml", i))
				err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
				Expect(err).NotTo(HaveOccurred())
				defer os.Remove(bsFile)

				cmd := exec.Command("kubectl", "apply", "-f", bsFile)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("verifying BatchSandbox %s allocates pods", newBatchSandboxName))
				Eventually(func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "batchsandbox", newBatchSandboxName, "-n", testNamespace,
						"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
					allocStatusJSON, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(allocStatusJSON).NotTo(BeEmpty())

					var allocStatus struct {
						Pods []string `json:"pods"`
					}
					err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(len(allocStatus.Pods)).To(Equal(replicasPerSandbox))
				}, 2*time.Minute).Should(Succeed())
			}

			By("cleaning up")
			for i := 0; i < numBatchSandboxes; i++ {
				cmd = exec.Command("kubectl", "delete", "batchsandbox",
					fmt.Sprintf("test-bs-batch-new-%d", i), "-n", testNamespace)
				_, _ = utils.Run(cmd)
			}
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})
	})
})

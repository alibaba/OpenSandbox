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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/test/utils"
)

// PodRecyclePolicy E2E tests for Issue #452
// Tests the pod disposal policy when pooled BatchSandbox is deleted.
// Two policies are supported:
// - Delete (default): Pod is deleted when BatchSandbox is deleted
// - Reuse: Pod is reset and returned to pool for reuse
var _ = Describe("PodRecyclePolicy", Ordered, func() {
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

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Delete Policy", func() {
		It("should delete pods when BatchSandbox is deleted with Delete policy", func() {
			const poolName = "test-pool-delete-policy"
			const batchSandboxName = "test-bs-delete-policy"

			By("creating a Pool with Delete policy (default)")
			poolYAML, err := renderTemplate("testdata/pool-delete-policy.yaml", map[string]interface{}{
				"PoolName":          poolName,
				"Namespace":         testNamespace,
				"TaskExecutorImage": utils.TaskExecutorImage,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-delete-policy.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("waiting for Pool to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).NotTo(BeEmpty())
			}, 2*time.Minute).Should(Succeed())

			By("creating a BatchSandbox using the pool")
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

			By("recording allocated pod names")
			var allocatedPodNames []string
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
				g.Expect(len(allocStatus.Pods)).To(BeNumerically(">", 0))
				allocatedPodNames = allocStatus.Pods
			}, 2*time.Minute).Should(Succeed())

			By("deleting the BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying pods are deleted (not returned to pool)")
			Eventually(func(g Gomega) {
				for _, podName := range allocatedPodNames {
					cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace)
					_, err := utils.Run(cmd)
					g.Expect(err).To(HaveOccurred(), "Pod %s should be deleted", podName)
					g.Expect(err.Error()).To(ContainSubstring("not found"))
				}
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up the Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should use Delete policy for existing pools without podRecyclePolicy field (backward compatibility)", func() {
			const poolName = "test-pool-compat"
			const batchSandboxName = "test-bs-compat"

			By("creating a Pool without podRecyclePolicy field (old behavior)")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    3,
				"BufferMin":    1,
				"PoolMax":      5,
				"PoolMin":      2,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-compat.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("waiting for Pool to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).NotTo(BeEmpty())
			}, 2*time.Minute).Should(Succeed())

			By("creating a BatchSandbox using the pool")
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         1,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-compat.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("recording allocated pod names")
			var allocatedPodNames []string
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
				g.Expect(len(allocStatus.Pods)).To(BeNumerically(">", 0))
				allocatedPodNames = allocStatus.Pods
			}, 2*time.Minute).Should(Succeed())

			By("deleting the BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying pods are deleted (default Delete policy for backward compatibility)")
			Eventually(func(g Gomega) {
				for _, podName := range allocatedPodNames {
					cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace)
					_, err := utils.Run(cmd)
					g.Expect(err).To(HaveOccurred(), "Pod %s should be deleted (default Delete policy)", podName)
					g.Expect(err.Error()).To(ContainSubstring("not found"))
				}
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up the Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Reuse Policy", func() {
		It("should reset and return pods to pool when BatchSandbox is deleted with Reuse policy", func() {
			const poolName = "test-pool-reuse-policy"
			const batchSandboxName = "test-bs-reuse-policy"

			By("creating a Pool with Reuse policy and task-executor sidecar")
			poolYAML, err := renderTemplate("testdata/pool-reuse-policy.yaml", map[string]interface{}{
				"PoolName":          poolName,
				"Namespace":         testNamespace,
				"TaskExecutorImage": utils.TaskExecutorImage,
				"PoolMax":           1, // Use poolMax=1 so there is only 1 pod, ensuring the same pod is reused
				"PoolMin":           1,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-reuse-policy.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("waiting for Pool to be ready and pods have task-executor sidecar")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).NotTo(BeEmpty())

				// Verify pods have the reuse-enabled label
				cmd = exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"-o", "jsonpath={.items[*].metadata.labels.pool\\.opensandbox\\.io/reuse-enabled}")
				reuseLabels, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(reuseLabels).To(ContainSubstring("true"), "Pods should have reuse-enabled=true label")
			}, 2*time.Minute).Should(Succeed())

			By("verifying pool pods have task-executor sidecar injected")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"-o", "jsonpath={.items[0].spec.containers[*].name}")
				containerNames, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(containerNames).To(ContainSubstring("task-executor"), "Pool pod should have task-executor sidecar")
			}, 30*time.Second).Should(Succeed())

			By("verifying pool pods have shareProcessNamespace=true")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"-o", "jsonpath={.items[0].spec.shareProcessNamespace}")
				shareNs, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(shareNs).To(Equal("true"), "Pool pod should have shareProcessNamespace=true for nsenter support")
			}, 30*time.Second).Should(Succeed())

			By("verifying task-executor container has SYS_PTRACE capability")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"-o", "jsonpath={.items[0].spec.containers[?(@.name=='task-executor')].securityContext.capabilities.add}")
				caps, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(caps).To(ContainSubstring("SYS_PTRACE"), "task-executor container should have SYS_PTRACE capability")
			}, 30*time.Second).Should(Succeed())

			By("recording initial pool total")
			cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
				"-o", "jsonpath={.status.total}")
			initialTotalStr, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a BatchSandbox using the pool")
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         1,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-reuse-policy.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("recording allocated pod names")
			var allocatedPodNames []string
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
				g.Expect(len(allocStatus.Pods)).To(BeNumerically(">", 0))
				allocatedPodNames = allocStatus.Pods
			}, 2*time.Minute).Should(Succeed())

			By("verifying BatchSandbox has pod-disposal finalizer")
			cmd = exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
				"-o", "jsonpath={.metadata.finalizers}")
			finalizers, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalizers).To(ContainSubstring("batch-sandbox.sandbox.opensandbox.io/pod-disposal"),
				"Pooled BatchSandbox should have pod-disposal finalizer")

			By("deleting the BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying pods still exist (returned to pool after reset)")
			Eventually(func(g Gomega) {
				for _, podName := range allocatedPodNames {
					cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
						"-o", "jsonpath={.status.phase}")
					phase, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred(), "Pod %s should still exist", podName)
					g.Expect(phase).To(Equal("Running"), "Pod %s should be Running", podName)
				}
			}, 2*time.Minute).Should(Succeed())

			By("verifying pool total remains the same (pods returned)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).To(Equal(initialTotalStr), "Pool total should remain the same after pod return")
			}, 30*time.Second).Should(Succeed())

			By("verifying pods can be reallocated to a new BatchSandbox (same pod reused)")
			// Create another BatchSandbox to verify the same pod is reused
			const batchSandboxName2 = "test-bs-reuse-policy-2"
			bsYAML2, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName2,
				"Namespace":        testNamespace,
				"Replicas":         1,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile2 := filepath.Join("/tmp", "test-bs-reuse-policy-2.yaml")
			err = os.WriteFile(bsFile2, []byte(bsYAML2), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile2)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile2)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var reallocatedPodNames []string
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
				g.Expect(len(allocStatus.Pods)).To(Equal(1), "Second BatchSandbox should allocate 1 pod")
				reallocatedPodNames = allocStatus.Pods
			}, 2*time.Minute).Should(Succeed())

			By("verifying the reallocated pod is the same pod that was previously reset (pod identity preserved)")
			Expect(reallocatedPodNames).To(HaveLen(1))
			Expect(allocatedPodNames).To(HaveLen(1))
			Expect(reallocatedPodNames[0]).To(Equal(allocatedPodNames[0]),
				"The same pod %s should be reused by the second BatchSandbox after reset",
				allocatedPodNames[0])

			By("cleaning up BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName2, "-n", testNamespace)
			_, _ = utils.Run(cmd)

			By("cleaning up the Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// ============================================================
// Case 2: Multiple Pods Concurrent Reset (includes pool.status.resetting verification)
// ============================================================

var _ = Describe("PodRecyclePolicy - Multiple Pods Reuse", Ordered, func() {
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
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Reuse policy with multiple pods", func() {
		It("should reset multiple pods concurrently and return all to pool", func() {
			const poolName = "test-pool-multi-pod"
			const batchSandboxName = "test-bs-multi-pod"

			By("creating a Pool with Reuse policy (poolMin=3)")
			poolYAML, err := renderTemplate("testdata/pool-reuse-multi-pod.yaml", map[string]interface{}{
				"PoolName":          poolName,
				"Namespace":         testNamespace,
				"TaskExecutorImage": utils.TaskExecutorImage,
				"PoolMin":           3,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-multi-pod.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("waiting for Pool total >= 3")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				total := 0
				fmt.Sscanf(totalStr, "%d", &total)
				g.Expect(total).To(BeNumerically(">=", 3))
			}, 3*time.Minute).Should(Succeed())

			By("recording initial pool total")
			cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
				"-o", "jsonpath={.status.total}")
			initialTotalStr, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a BatchSandbox with replicas=3")
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         3,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-multi-pod.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for all 3 pods to be allocated")
			var allocatedPodNames []string
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
				g.Expect(len(allocStatus.Pods)).To(Equal(3), "Should have exactly 3 pods allocated")
				allocatedPodNames = allocStatus.Pods
			}, 3*time.Minute).Should(Succeed())

			By("deleting the BatchSandbox to trigger concurrent reset for all 3 pods")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Try to observe resetting > 0 (best-effort, the window may be short)
			By("attempting to observe pool.status.resetting > 0 during reset (best-effort)")
			resettingObserved := false
			for i := 0; i < 10; i++ {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.resetting}")
				resettingStr, err := utils.Run(cmd)
				if err == nil {
					resetting := 0
					fmt.Sscanf(resettingStr, "%d", &resetting)
					if resetting > 0 {
						resettingObserved = true
						break
					}
				}
				time.Sleep(500 * time.Millisecond)
			}
			if resettingObserved {
				By("observed pool.status.resetting > 0 during reset")
			} else {
				By("reset completed too quickly to observe resetting > 0, skipping intermediate check")
			}

			By("verifying all 3 pods still exist and are Running after reset")
			Eventually(func(g Gomega) {
				for _, podName := range allocatedPodNames {
					cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
						"-o", "jsonpath={.status.phase}")
					phase, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred(), "Pod %s should still exist after reset", podName)
					g.Expect(phase).To(Equal("Running"), "Pod %s should be Running after reset", podName)
				}
			}, 3*time.Minute).Should(Succeed())

			By("verifying pool total remains the same (all pods returned to pool)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).To(Equal(initialTotalStr), "Pool total should remain unchanged after concurrent reset")
			}, 30*time.Second).Should(Succeed())

			By("verifying pool.status.resetting == 0 after reset completes")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.resetting}")
				resettingStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// Empty string means 0 (omitempty field)
				if resettingStr != "" {
					resetting := 0
					fmt.Sscanf(resettingStr, "%d", &resetting)
					g.Expect(resetting).To(Equal(0), "pool.status.resetting should be 0 after reset completes")
				}
			}, 3*time.Minute).Should(Succeed())

			By("verifying pool available >= 3 (all pods available again)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				availStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				avail := 0
				fmt.Sscanf(availStr, "%d", &avail)
				g.Expect(avail).To(BeNumerically(">=", 3), "All reset pods should be available in pool")
			}, 30*time.Second).Should(Succeed())

			By("cleaning up the Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// Helper function to check if string slice contains a string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// Helper function to get non-empty lines from a string
func getNonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

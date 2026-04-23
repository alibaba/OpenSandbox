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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/test/utils"
)

// namespace where the project is deployed in
const namespace = "opensandbox-system"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("CONTROLLER_IMG=%s", utils.ControllerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				goTemplate := `{{ range .items }}` +
					`{{ if not .metadata.deletionTimestamp }}` +
					`{{ .metadata.name }}` +
					`{{ "\n" }}{{ end }}{{ end }}`
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template="+goTemplate,
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})
	})

	Context("Pool", func() {
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

		It("should handle pod eviction correctly", func() {
			const poolName = "test-pool-eviction"
			const testNamespace = "default"

			By("creating a Pool")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    4,
				"BufferMin":    2,
				"PoolMax":      6,
				"PoolMin":      3,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", poolName+".yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool pods to be Running")
			var allPoolPods []string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"--field-selector=status.phase=Running",
					"-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				allPoolPods = strings.Fields(output)
				g.Expect(len(allPoolPods)).To(BeNumerically(">=", 3))
			}, 3*time.Minute).Should(Succeed())

			By("allocating pods via BatchSandbox")
			const batchSandboxName = "test-bs-eviction"
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         2,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", batchSandboxName+".yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var allocatedPods []string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				out, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).NotTo(BeEmpty())

				var alloc struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(out), &alloc)
				g.Expect(err).NotTo(HaveOccurred())
				allocatedPods = alloc.Pods
				g.Expect(allocatedPods).To(HaveLen(2))
			}, 2*time.Minute).Should(Succeed())

			By("marking all pool pods for eviction")
			for _, pod := range allPoolPods {
				cmd := exec.Command("kubectl", "label", "pod", pod, "-n", testNamespace,
					"pool.opensandbox.io/evict=true", "--overwrite")
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
			}

			By("verifying allocated pods are not deleted")
			Consistently(func(g Gomega) {
				for _, pod := range allocatedPods {
					cmd := exec.Command("kubectl", "get", "pod", pod, "-n", testNamespace,
						"-o", "jsonpath={.metadata.deletionTimestamp}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred(), "allocated pod %s should still exist", pod)
					g.Expect(output).To(BeEmpty(), "allocated pod %s should not be terminating", pod)
				}
			}, 30*time.Second, 3*time.Second).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})

		It("should evict idle pool pods", func() {
			const poolName = "test-pool-eviction-b"
			const testNamespace = "default"

			By("creating a Pool")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    3,
				"BufferMin":    2,
				"PoolMax":      5,
				"PoolMin":      3,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", poolName+".yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pool to stabilise with idle pods")
			var idlePods []string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"--field-selector=status.phase=Running",
					"-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				idlePods = strings.Fields(output)
				g.Expect(len(idlePods)).To(BeNumerically(">=", 2))
			}, 3*time.Minute).Should(Succeed())

			By("marking all pool pods for eviction")
			for _, pod := range idlePods {
				cmd := exec.Command("kubectl", "label", "pod", pod, "-n", testNamespace,
					"pool.opensandbox.io/evict=true", "--overwrite")
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
			}

			By("verifying all idle pods are eventually deleted")
			Eventually(func(g Gomega) {
				for _, pod := range idlePods {
					cmd := exec.Command("kubectl", "get", "pod", pod, "-n", testNamespace,
						"-o", "jsonpath={.metadata.deletionTimestamp}")
					output, err := utils.Run(cmd)
					if err != nil && strings.Contains(err.Error(), "not found") {
						continue // already gone
					}
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).NotTo(BeEmpty(), "idle pod %s should be terminating", pod)
				}
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})

		It("should not allocate pods marked for eviction to a BatchSandbox", func() {
			const poolName = "test-pool-eviction-c"
			const testNamespace = "default"

			By("creating a Pool")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    4,
				"BufferMin":    2,
				"PoolMax":      6,
				"PoolMin":      3,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", poolName+".yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to replenish with fresh idle pods")
			var freshPods []string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"--field-selector=status.phase=Running",
					"-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				freshPods = strings.Fields(output)
				g.Expect(len(freshPods)).To(BeNumerically(">=", 2))
			}, 3*time.Minute).Should(Succeed())

			By("marking all current idle pods for eviction before any BatchSandbox claims them")
			for _, pod := range freshPods {
				cmd := exec.Command("kubectl", "label", "pod", pod, "-n", testNamespace,
					"pool.opensandbox.io/evict=true", "--overwrite")
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
			}
			evictingPods := make(map[string]bool)
			for _, pod := range freshPods {
				evictingPods[pod] = true
			}

			By("creating a BatchSandbox that requests pods from the pool")
			const batchSandboxName = "test-bs-eviction-c"
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         1,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", batchSandboxName+".yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying BatchSandbox gets a pod that is NOT one of the evicting pods")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				out, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).NotTo(BeEmpty())

				var alloc struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(out), &alloc)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(alloc.Pods).To(HaveLen(1))
				g.Expect(evictingPods).NotTo(HaveKey(alloc.Pods[0]),
					"evicting pod %s should not be allocated", alloc.Pods[0])
			}, 3*time.Minute).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})

		It("should correctly create pods and maintain pool status", func() {
			const poolName = "test-pool-basic"
			const testNamespace = "default"
			const poolMin = 2
			const poolMax = 5
			const bufferMin = 1
			const bufferMax = 3

			By("creating a basic Pool")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    bufferMax,
				"BufferMin":    bufferMin,
				"PoolMax":      poolMax,
				"PoolMin":      poolMin,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-basic.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("verifying Pool creates pods and maintains correct status")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status}")
				statusOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(statusOutput).To(ContainSubstring(`"total":`), "Pool status should have total field")
				g.Expect(statusOutput).To(ContainSubstring(`"allocated":`), "Pool status should have allocated field")
				g.Expect(statusOutput).To(ContainSubstring(`"available":`), "Pool status should have available field")

				cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				total := 0
				if totalStr != "" {
					fmt.Sscanf(totalStr, "%d", &total)
				}
				g.Expect(total).To(BeNumerically(">=", poolMin), "Pool total should be >= poolMin")
				g.Expect(total).To(BeNumerically("<=", poolMax), "Pool total should be <= poolMax")
			}, 2*time.Minute).Should(Succeed())

			By("verifying pods are created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "Pool should create pods")
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up the Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should correctly manage capacity when poolMin and poolMax change", func() {
			const poolName = "test-pool-capacity"
			const testNamespace = "default"

			By("creating a Pool with initial capacity")
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

			poolFile := filepath.Join("/tmp", "test-pool-capacity.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for initial Pool to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				total := 0
				if totalStr != "" {
					fmt.Sscanf(totalStr, "%d", &total)
				}
				g.Expect(total).To(BeNumerically(">=", 2))
			}, 2*time.Minute).Should(Succeed())

			By("increasing poolMin to trigger scale up")
			poolYAML, err = renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    3,
				"BufferMin":    1,
				"PoolMax":      10,
				"PoolMin":      5,
			})
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Pool scales up to meet new poolMin")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				total := 0
				if totalStr != "" {
					fmt.Sscanf(totalStr, "%d", &total)
				}
				g.Expect(total).To(BeNumerically(">=", 5), "Pool should scale up to meet poolMin=5")
				g.Expect(total).To(BeNumerically("<=", 10), "Pool should not exceed poolMax=10")
			}, 2*time.Minute).Should(Succeed())

			By("decreasing poolMax to below current total")
			poolYAML, err = renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    2,
				"BufferMin":    1,
				"PoolMax":      3,
				"PoolMin":      2,
			})
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Pool respects new poolMax constraint")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				total := 0
				if totalStr != "" {
					fmt.Sscanf(totalStr, "%d", &total)
				}
				g.Expect(total).To(BeNumerically("<=", 3), "Pool should scale down to meet poolMax=3")
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up the Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should upgrade pool template correctly", func() {
			const poolName = "test-pool-upgrade"
			const testNamespace = "default"
			const batchSandboxName = "test-bs-for-upgrade"

			By("creating a Pool with initial template")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    3,
				"BufferMin":    2,
				"PoolMax":      5,
				"PoolMin":      2,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-upgrade.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).NotTo(BeEmpty())
			}, 2*time.Minute).Should(Succeed())

			By("allocating a pod from the pool via BatchSandbox")
			batchSandboxYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         1,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-upgrade.yaml")
			err = os.WriteFile(bsFile, []byte(batchSandboxYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for BatchSandbox to allocate pod")
			var allocatedPodNames []string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))

				cmd = exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				allocStatusJSON, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(allocStatusJSON).NotTo(BeEmpty(), "alloc-status annotation should exist")

				var allocStatus struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
				g.Expect(err).NotTo(HaveOccurred())

				allocatedPodNames = allocStatus.Pods
				g.Expect(len(allocatedPodNames)).To(Equal(1), "Should have 1 allocated pod")
			}, 2*time.Minute).Should(Succeed())

			By("getting all pool pods")
			cmd = exec.Command("kubectl", "get", "pods", "-n", testNamespace,
				"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
				"-o", "jsonpath={.items[*].metadata.name}")
			allPoolPodsStr, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			allPoolPods := strings.Fields(allPoolPodsStr)

			By("calculating available pods (all pool pods - allocated pods)")
			availablePodsBeforeUpgrade := []string{}
			allocatedPodMap := make(map[string]bool)
			for _, podName := range allocatedPodNames {
				allocatedPodMap[podName] = true
			}
			for _, podName := range allPoolPods {
				if !allocatedPodMap[podName] {
					availablePodsBeforeUpgrade = append(availablePodsBeforeUpgrade, podName)
				}
			}

			By("updating Pool template with new environment variable")
			updatedPoolYAML, err := renderTemplate("testdata/pool-with-env.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"Namespace":    testNamespace,
				"SandboxImage": utils.SandboxImage,
				"BufferMax":    3,
				"BufferMin":    2,
				"PoolMax":      5,
				"PoolMin":      2,
			})
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(poolFile, []byte(updatedPoolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying allocated pod is NOT upgraded")
			Consistently(func(g Gomega) {
				for _, allocatedPod := range allocatedPodNames {
					cmd := exec.Command("kubectl", "get", "pod", allocatedPod, "-n", testNamespace,
						"-o", "jsonpath={.metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal(allocatedPod), "Allocated pod should not be recreated")
				}
			}, 30*time.Second, 3*time.Second).Should(Succeed())

			By("verifying available pods are recreated with new template")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"-o", "jsonpath={.items[*].metadata.name}")
				allPodsAfterStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				allPodsAfter := strings.Fields(allPodsAfterStr)

				// Get currently allocated pods
				cmd = exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				allocStatusJSON, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var allocStatus struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
				g.Expect(err).NotTo(HaveOccurred())

				currentAllocatedPods := make(map[string]bool)
				for _, podName := range allocStatus.Pods {
					currentAllocatedPods[podName] = true
				}

				// Calculate available pods after upgrade
				availablePodsAfterUpgrade := []string{}
				for _, podName := range allPodsAfter {
					if !currentAllocatedPods[podName] {
						availablePodsAfterUpgrade = append(availablePodsAfterUpgrade, podName)
					}
				}

				// Check if at least one available pod was recreated
				recreated := false
				for _, oldPod := range availablePodsBeforeUpgrade {
					found := false
					for _, newPod := range availablePodsAfterUpgrade {
						if oldPod == newPod {
							found = true
							break
						}
					}
					if !found {
						recreated = true
						break
					}
				}
				g.Expect(recreated).To(BeTrue(), "At least one available pod should be recreated")
			}, 3*time.Minute).Should(Succeed())

			By("verifying new pods have the upgraded environment variable")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var podList struct {
					Items []struct {
						Metadata struct {
							Name string `json:"name"`
						} `json:"metadata"`
						Spec struct {
							Containers []struct {
								Name string `json:"name"`
								Env  []struct {
									Name  string `json:"name"`
									Value string `json:"value"`
								} `json:"env"`
							} `json:"containers"`
						} `json:"spec"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &podList)
				g.Expect(err).NotTo(HaveOccurred())

				// Get currently allocated pods
				cmd = exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				allocStatusJSON, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var allocStatus struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
				g.Expect(err).NotTo(HaveOccurred())

				allocatedPodMap := make(map[string]bool)
				for _, podName := range allocStatus.Pods {
					allocatedPodMap[podName] = true
				}

				// Find at least one available pod with UPGRADED=true
				foundUpgraded := false
				for _, pod := range podList.Items {
					if !allocatedPodMap[pod.Metadata.Name] {
						// This is an available pod
						for _, container := range pod.Spec.Containers {
							if container.Name == "sandbox-container" {
								for _, env := range container.Env {
									if env.Name == "UPGRADED" && env.Value == "true" {
										foundUpgraded = true
										break
									}
								}
							}
						}
					}
				}
				g.Expect(foundUpgraded).To(BeTrue(), "At least one available pod should have UPGRADED=true env var")
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up BatchSandbox and Pool")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, _ = utils.Run(cmd)

			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("BatchSandbox", func() {
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

		It("should work correctly in non-pooled mode", func() {
			const batchSandboxName = "test-bs-non-pooled"
			const testNamespace = "default"
			const replicas = 2

			By("creating a non-pooled BatchSandbox")
			bsYAML, err := renderTemplate("testdata/batchsandbox-non-pooled.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"SandboxImage":     utils.SandboxImage,
				"Namespace":        testNamespace,
				"Replicas":         replicas,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-non-pooled.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd := exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying pods are created directly from template")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var podList struct {
					Items []struct {
						Metadata struct {
							Name            string `json:"name"`
							OwnerReferences []struct {
								Kind string `json:"kind"`
								Name string `json:"name"`
								UID  string `json:"uid"`
							} `json:"ownerReferences"`
						} `json:"metadata"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &podList)
				g.Expect(err).NotTo(HaveOccurred())

				// Find pods owned by this BatchSandbox
				ownedPods := []string{}
				for _, pod := range podList.Items {
					for _, owner := range pod.Metadata.OwnerReferences {
						if owner.Kind == "BatchSandbox" && owner.Name == batchSandboxName {
							ownedPods = append(ownedPods, pod.Metadata.Name)
							break
						}
					}
				}
				g.Expect(len(ownedPods)).To(Equal(replicas), "Should create %d pods", replicas)
			}, 2*time.Minute).Should(Succeed())

			By("verifying BatchSandbox status is correctly updated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status}")
				statusOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statusOutput).To(ContainSubstring(fmt.Sprintf(`"replicas":%d`, replicas)))
				g.Expect(statusOutput).To(ContainSubstring(fmt.Sprintf(`"allocated":%d`, replicas)))
				g.Expect(statusOutput).To(ContainSubstring(fmt.Sprintf(`"ready":%d`, replicas)))
			}, 2*time.Minute).Should(Succeed())

			By("verifying endpoint annotation is set")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/endpoints}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty())
				endpoints := strings.Split(output, ",")
				g.Expect(len(endpoints)).To(Equal(replicas))
			}, 30*time.Second).Should(Succeed())

			By("cleaning up BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying pods are deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace, "-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var podList struct {
					Items []struct {
						Metadata struct {
							Name              string  `json:"name"`
							DeletionTimestamp *string `json:"deletionTimestamp"`
							OwnerReferences   []struct {
								Kind string `json:"kind"`
								Name string `json:"name"`
							} `json:"ownerReferences"`
						} `json:"metadata"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &podList)
				g.Expect(err).NotTo(HaveOccurred())

				// Check no pods are owned by this BatchSandbox or they have deletionTimestamp
				for _, pod := range podList.Items {
					for _, owner := range pod.Metadata.OwnerReferences {
						if owner.Kind == "BatchSandbox" && owner.Name == batchSandboxName {
							g.Expect(pod.Metadata.DeletionTimestamp).NotTo(BeNil(),
								"Pod %s owned by BatchSandbox should have deletionTimestamp set", pod.Metadata.Name)
						}
					}
				}
			}, 2*time.Minute).Should(Succeed())
		})

		It("should work correctly in pooled mode", func() {
			const poolName = "test-pool-for-bs"
			const batchSandboxName = "test-bs-pooled"
			const testNamespace = "default"
			const replicas = 2

			By("creating a Pool")
			// Use a zero-buffer pool so restart recovery verifies allocation reconstruction
			// without racing the normal buffer refill behavior.
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    0,
				"BufferMin":    0,
				"PoolMax":      5,
				"PoolMin":      2,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-for-bs.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).NotTo(BeEmpty())
			}, 2*time.Minute).Should(Succeed())

			By("creating a pooled BatchSandbox")
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"SandboxImage":     utils.SandboxImage,
				"Namespace":        testNamespace,
				"Replicas":         replicas,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-pooled.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying BatchSandbox allocates pods from pool")
			Eventually(func(g Gomega) {
				// Verify alloc-status annotation contains pool pod names
				cmd = exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				allocStatusJSON, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(allocStatusJSON).NotTo(BeEmpty(), "alloc-status annotation should exist")

				var allocStatus struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(allocStatus.Pods)).To(Equal(replicas), "Should have %d pods in alloc-status", replicas)

				// Verify the pods in alloc-status are from the pool
				for _, podName := range allocStatus.Pods {
					cmd = exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
						"-o", "jsonpath={.metadata.labels.sandbox\\.opensandbox\\.io/pool-name}")
					poolLabel, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(poolLabel).To(Equal(poolName), "Pod %s should be from pool %s", podName, poolName)
				}
			}, 2*time.Minute).Should(Succeed())

			By("verifying BatchSandbox status is correctly updated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status}")
				statusOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statusOutput).To(ContainSubstring(fmt.Sprintf(`"replicas":%d`, replicas)))
				g.Expect(statusOutput).To(ContainSubstring(fmt.Sprintf(`"ready":%d`, replicas)))
			}, 30*time.Second).Should(Succeed())

			By("verifying endpoint annotation is set")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/endpoints}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty())
				endpoints := strings.Split(output, ",")
				g.Expect(len(endpoints)).To(Equal(replicas))
			}, 30*time.Second).Should(Succeed())

			By("recording Pool allocated count")
			cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
				"-o", "jsonpath={.status.allocated}")
			allocatedBefore, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("cleaning up BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying pods are returned to pool")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				allocatedAfter, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				before := 0
				if allocatedBefore != "" {
					fmt.Sscanf(allocatedBefore, "%d", &before)
				}
				after := 0
				if allocatedAfter != "" {
					fmt.Sscanf(allocatedAfter, "%d", &after)
				}
				g.Expect(after).To(BeNumerically("<", before), "Allocated count should decrease")
			}, 30*time.Second).Should(Succeed())

			By("cleaning up Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should expire and delete non-pooled BatchSandbox correctly", func() {
			const batchSandboxName = "test-bs-expire-non-pooled"
			const testNamespace = "default"
			const replicas = 1

			By("creating a non-pooled BatchSandbox with expireTime")
			expireTime := time.Now().Add(45 * time.Second).UTC().Format(time.RFC3339)

			bsYAML, err := renderTemplate("testdata/batchsandbox-non-pooled-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         replicas,
				"ExpireTime":       expireTime,
				"SandboxImage":     utils.SandboxImage,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-expire-non-pooled.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd := exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying BatchSandbox is created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(fmt.Sprintf("%d", replicas)))
			}, 2*time.Minute).Should(Succeed())

			By("recording pod names")
			cmd = exec.Command("kubectl", "get", "pods", "-n", testNamespace, "-o", "json")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var podList struct {
				Items []struct {
					Metadata struct {
						Name            string `json:"name"`
						OwnerReferences []struct {
							Kind string `json:"kind"`
							Name string `json:"name"`
						} `json:"ownerReferences"`
					} `json:"metadata"`
				} `json:"items"`
			}
			err = json.Unmarshal([]byte(output), &podList)
			Expect(err).NotTo(HaveOccurred())

			podNamesList := []string{}
			for _, pod := range podList.Items {
				for _, owner := range pod.Metadata.OwnerReferences {
					if owner.Kind == "BatchSandbox" && owner.Name == batchSandboxName {
						podNamesList = append(podNamesList, pod.Metadata.Name)
						break
					}
				}
			}
			Expect(len(podNamesList)).To(BeNumerically(">", 0), "Should have pods owned by BatchSandbox")

			By("waiting for BatchSandbox to expire and be deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("not found"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying pods are deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace, "-o", "json")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var currentPodList struct {
					Items []struct {
						Metadata struct {
							Name              string  `json:"name"`
							DeletionTimestamp *string `json:"deletionTimestamp"`
							OwnerReferences   []struct {
								Kind string `json:"kind"`
								Name string `json:"name"`
							} `json:"ownerReferences"`
						} `json:"metadata"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &currentPodList)
				g.Expect(err).NotTo(HaveOccurred())

				// Verify no pods are owned by the deleted BatchSandbox or they have deletionTimestamp
				for _, pod := range currentPodList.Items {
					for _, owner := range pod.Metadata.OwnerReferences {
						if owner.Kind == "BatchSandbox" && owner.Name == batchSandboxName {
							g.Expect(pod.Metadata.DeletionTimestamp).NotTo(BeNil(),
								"Pod %s owned by BatchSandbox should have deletionTimestamp set", pod.Metadata.Name)
						}
					}
				}
			}, 30*time.Second).Should(Succeed())
		})

		It("should expire and return pooled BatchSandbox pods to pool", func() {
			const poolName = "test-pool-for-expire"
			const batchSandboxName = "test-bs-expire-pooled"
			const testNamespace = "default"
			const replicas = 1

			By("creating a Pool")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    3,
				"BufferMin":    2,
				"PoolMax":      5,
				"PoolMin":      2,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-for-expire.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).NotTo(BeEmpty())
			}, 2*time.Minute).Should(Succeed())

			By("recording Pool allocated count before BatchSandbox creation")
			cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
				"-o", "jsonpath={.status.allocated}")
			allocatedBeforeBS, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a pooled BatchSandbox with expireTime")
			expireTime := time.Now().Add(45 * time.Second).UTC().Format(time.RFC3339)
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"SandboxImage":     utils.SandboxImage,
				"Namespace":        testNamespace,
				"Replicas":         replicas,
				"PoolName":         poolName,
				"ExpireTime":       expireTime,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-expire-pooled.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("recording pod names from alloc-status")
			var podNamesList []string
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
				podNamesList = allocStatus.Pods
				g.Expect(len(podNamesList)).To(BeNumerically(">", 0), "Should have allocated pods")
			}, 2*time.Minute).Should(Succeed())

			allocatedAfterBS := ""
			By("verifying Pool allocated count increased after BatchSandbox allocation")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				_allocatedAfterBS, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				allocatedAfterBS = _allocatedAfterBS

				before := 0
				if allocatedBeforeBS != "" {
					fmt.Sscanf(allocatedBeforeBS, "%d", &before)
				}

				after := 0
				if _allocatedAfterBS != "" {
					fmt.Sscanf(allocatedAfterBS, "%d", &after)
				}

				g.Expect(after).To(BeNumerically(">", before), "Pool allocated count should increase after BatchSandbox allocation")
			}, 30*time.Second).Should(Succeed())

			By("waiting for BatchSandbox to expire and be deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("not found"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying pods still exist and are returned to pool")
			Eventually(func(g Gomega) {
				for _, podName := range podNamesList {
					cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
						"-o", "jsonpath={.metadata.name}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal(podName), "Pod should still exist")
				}
			}, 30*time.Second).Should(Succeed())

			By("verifying Pool allocated count decreased after BatchSandbox expiration")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				allocatedAfterExpiration, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				before := 0
				if allocatedAfterBS != "" {
					fmt.Sscanf(allocatedAfterBS, "%d", &before)
				}
				after := 0
				if allocatedAfterExpiration != "" {
					fmt.Sscanf(allocatedAfterExpiration, "%d", &after)
				}
				g.Expect(after).To(BeNumerically("<", before), "Allocated count should decrease")
			}, 30*time.Second).Should(Succeed())

			By("cleaning up Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Task", func() {
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

		It("should successfully manage Pool with task scheduling", func() {
			const poolName = "test-pool"
			const batchSandboxName = "test-batchsandbox-with-task"
			const testNamespace = "default"
			const replicas = 2

			By("creating a Pool with task-executor sidecar")
			poolTemplateFile := filepath.Join("testdata", "pool-with-task-executor.yaml")
			poolYAML, err := renderTemplate(poolTemplateFile, map[string]interface{}{
				"PoolName":          poolName,
				"Namespace":         testNamespace,
				"TaskExecutorImage": utils.TaskExecutorImage,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("waiting for Pool to be ready")
			verifyPoolReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				output, err := utils.Run(cmd)
				By(fmt.Sprintf("waiting for Pool to be ready, output %s", output))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "Pool status.total should not be empty")
			}
			Eventually(verifyPoolReady, 2*time.Minute).Should(Succeed())

			By("creating a BatchSandbox with process-based tasks using the Pool")
			batchSandboxTemplateFile := filepath.Join("testdata", "batchsandbox-with-process-task.yaml")
			batchSandboxYAML, err := renderTemplate(batchSandboxTemplateFile, map[string]interface{}{
				"BatchSandboxName":  batchSandboxName,
				"Namespace":         testNamespace,
				"Replicas":          replicas,
				"PoolName":          poolName,
				"TaskExecutorImage": utils.TaskExecutorImage,
			})
			Expect(err).NotTo(HaveOccurred())

			batchSandboxFile := filepath.Join("/tmp", "test-batchsandbox.yaml")
			err = os.WriteFile(batchSandboxFile, []byte(batchSandboxYAML), 0644)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", batchSandboxFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create BatchSandbox")

			By("verifying BatchSandbox successfully allocated endpoints")
			verifyBatchSandboxAllocated := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(fmt.Sprintf("%d", replicas)), "BatchSandbox should allocate %d replicas", replicas)
			}
			Eventually(verifyBatchSandboxAllocated, 2*time.Minute).Should(Succeed())

			By("verifying BatchSandbox endpoints are available")
			verifyEndpoints := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/endpoints}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "BatchSandbox should have sandbox.opensandbox.io/endpoints annotation")
				endpoints := strings.Split(output, ",")
				g.Expect(len(endpoints)).To(Equal(replicas), "Should have %d endpoints", replicas)
			}
			Eventually(verifyEndpoints, 30*time.Second).Should(Succeed())

			By("verifying BatchSandbox status is as expected")
			verifyBatchSandboxStatus := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status}")
				statusOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statusOutput).To(ContainSubstring(fmt.Sprintf(`"replicas":%d`, replicas)))
				g.Expect(statusOutput).To(ContainSubstring(fmt.Sprintf(`"allocated":%d`, replicas)))
				g.Expect(statusOutput).To(ContainSubstring(fmt.Sprintf(`"ready":%d`, replicas)))
			}
			Eventually(verifyBatchSandboxStatus, 30*time.Second).Should(Succeed())

			By("verifying all tasks are successfully scheduled and succeeded")
			verifyTasksSucceeded := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status.taskSucceed}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(fmt.Sprintf("%d", replicas)), "All tasks should succeed")

				cmd = exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status.taskFailed}")
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("0"), "No tasks should fail")
			}
			Eventually(verifyTasksSucceeded, 2*time.Minute).Should(Succeed())

			By("recording Pool status before deletion")
			cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
				"-o", "jsonpath={.status.allocated}")
			poolAllocatedBefore, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("deleting the BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete BatchSandbox")

			By("verifying all tasks are unloaded and BatchSandbox is deleted")
			verifyBatchSandboxDeleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "BatchSandbox should be deleted")
				g.Expect(err.Error()).To(ContainSubstring("not found"))
			}
			Eventually(verifyBatchSandboxDeleted, 2*time.Minute).Should(Succeed())

			By("verifying pods are returned to the Pool")
			verifyPodsReturnedToPool := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				poolAllocatedAfter, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				beforeCount := 0
				if poolAllocatedBefore != "" {
					fmt.Sscanf(poolAllocatedBefore, "%d", &beforeCount)
				}
				afterCount := 0
				if poolAllocatedAfter != "" {
					fmt.Sscanf(poolAllocatedAfter, "%d", &afterCount)
				}
				g.Expect(afterCount).To(BeNumerically("<=", beforeCount),
					"Pool allocated count should decrease or stay same after BatchSandbox deletion")
			}
			Eventually(verifyPodsReturnedToPool, 30*time.Second).Should(Succeed())

			By("cleaning up the Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete Pool")

			By("cleaning up temporary files")
			os.Remove(poolFile)
			os.Remove(batchSandboxFile)
		})
	})

	Context("Pool Update", func() {
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

		It("should perform rolling update with maxUnavailable constraint", func() {
			const poolName = "test-pool-rolling-update"
			const testNamespace = "default"
			const poolSize = 10
			const maxUnavailablePercent = "20%"

			By("creating a Pool with updateStrategy")
			poolYAML, err := renderTemplate("testdata/pool-with-update-strategy.yaml", map[string]interface{}{
				"PoolName":       poolName,
				"SandboxImage":   utils.SandboxImage,
				"Namespace":      testNamespace,
				"BufferMax":      poolSize,
				"BufferMin":      poolSize - 2,
				"PoolMax":        poolSize,
				"PoolMin":        poolSize,
				"MaxUnavailable": maxUnavailablePercent,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", poolName+".yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to have all pods running")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				total := 0
				fmt.Sscanf(totalStr, "%d", &total)
				g.Expect(total).To(Equal(poolSize))

				cmd = exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"--field-selector=status.phase=Running",
					"-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(strings.Fields(output))).To(Equal(poolSize))
			}, 3*time.Minute).Should(Succeed())

			By("allocating some pods via BatchSandbox")
			const batchSandboxName = "test-bs-rolling-update"
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         3,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", batchSandboxName+".yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var allocatedPods []string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				out, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).NotTo(BeEmpty())

				var alloc struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(out), &alloc)
				g.Expect(err).NotTo(HaveOccurred())
				allocatedPods = alloc.Pods
				g.Expect(allocatedPods).To(HaveLen(3))
			}, 2*time.Minute).Should(Succeed())

			By("recording initial revision from pool status")
			cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
				"-o", "jsonpath={.status.revision}")
			initialRevision, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("triggering pool update by changing template")
			updatedPoolYAML, err := renderTemplate("testdata/pool-with-update-strategy.yaml", map[string]interface{}{
				"PoolName":       poolName,
				"SandboxImage":   utils.SandboxImage,
				"Namespace":      testNamespace,
				"BufferMax":      poolSize,
				"BufferMin":      poolSize - 2,
				"PoolMax":        poolSize,
				"PoolMin":        poolSize,
				"MaxUnavailable": maxUnavailablePercent,
				"EnvValue":       "v2",
			})
			Expect(err).NotTo(HaveOccurred())

			updatedPoolWithEnv := strings.Replace(updatedPoolYAML, "command: [\"sleep\", \"3600\"]",
				"command: [\"sleep\", \"3600\"]\n        env:\n        - name: VERSION\n          value: \"v2\"", 1)
			err = os.WriteFile(poolFile, []byte(updatedPoolWithEnv), 0644)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying allocated pods are not deleted during upgrade")
			Consistently(func(g Gomega) {
				for _, pod := range allocatedPods {
					cmd := exec.Command("kubectl", "get", "pod", pod, "-n", testNamespace,
						"-o", "jsonpath={.metadata.deletionTimestamp}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred(), "allocated pod %s should still exist", pod)
					g.Expect(output).To(BeEmpty(), "allocated pod %s should not be terminating", pod)
				}
			}, 60*time.Second, 5*time.Second).Should(Succeed())

			By("verifying new BatchSandbox can be allocated during upgrade")
			const newBatchSandboxName = "test-bs-rolling-update-new"
			newBSYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": newBatchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         1,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			newBSFile := filepath.Join("/tmp", newBatchSandboxName+".yaml")
			err = os.WriteFile(newBSFile, []byte(newBSYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(newBSFile)

			cmd = exec.Command("kubectl", "apply", "-f", newBSFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", newBatchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 2*time.Minute).Should(Succeed())

			By("verifying maxUnavailable constraint during upgrade")
			maxUnavailable := int(float64(poolSize) * 0.2)
			if maxUnavailable < 1 {
				maxUnavailable = 1
			}

			// Check a few times that unavailable pods don't exceed maxUnavailable
			for i := 0; i < 5; i++ {
				cmd := exec.Command("kubectl", "get", "pods", "-n", testNamespace,
					"-l", fmt.Sprintf("sandbox.opensandbox.io/pool-name=%s", poolName),
					"-o", "json")
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())

				var podList struct {
					Items []struct {
						Status struct {
							Phase      string `json:"phase"`
							Conditions []struct {
								Type   string `json:"type"`
								Status string `json:"status"`
							} `json:"conditions"`
						} `json:"status"`
					} `json:"items"`
				}
				err = json.Unmarshal([]byte(output), &podList)
				Expect(err).NotTo(HaveOccurred())

				unavailableCount := 0
				for _, pod := range podList.Items {
					if pod.Status.Phase != "Running" {
						unavailableCount++
						continue
					}
					ready := false
					for _, cond := range pod.Status.Conditions {
						if cond.Type == "Ready" && cond.Status == "True" {
							ready = true
							break
						}
					}
					if !ready {
						unavailableCount++
					}
				}
				Expect(unavailableCount).To(BeNumerically("<=", maxUnavailable+1),
					"unavailable pods should not exceed maxUnavailable + 1 (allowing for timing)")
				time.Sleep(2 * time.Second)
			}

			By("verifying pool status reflects upgrade progress")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.updated}")
				updatedStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				updated := 0
				if updatedStr != "" {
					fmt.Sscanf(updatedStr, "%d", &updated)
				}
				// At least some pods should be updated
				g.Expect(updated).To(BeNumerically(">", 0), "some pods should be updated")

				// Revision should be different from initial
				cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.revision}")
				currentRevision, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(currentRevision).NotTo(Equal(initialRevision))
			}, 2*time.Minute).Should(Succeed())

			By("releasing BatchSandbox to allow full upgrade")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "batchsandbox", newBatchSandboxName, "-n", testNamespace)
			_, _ = utils.Run(cmd)

			By("verifying pool eventually completes upgrade")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.updated}")
				updatedStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				updated := 0
				if updatedStr != "" {
					fmt.Sscanf(updatedStr, "%d", &updated)
				}
				g.Expect(updated).To(Equal(poolSize), "all pods should be updated")
			}, 3*time.Minute).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})
	})

	Context("Pool State Recovery", func() {
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

		It("should reconstruct pool allocation state after controller restart", func() {
			const poolName = "test-pool-recovery"
			const batchSandboxName = "test-bs-recovery"
			const testNamespace = "default"
			const replicas = 2

			By("creating a Pool")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    3,
				"BufferMin":    2,
				"PoolMax":      5,
				"PoolMin":      2,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-recovery.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).NotTo(BeEmpty())
			}, 2*time.Minute).Should(Succeed())

			By("creating a BatchSandbox that allocates from the pool")
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         replicas,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-recovery.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for BatchSandbox to allocate pods")
			var poolAllocatedBefore string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(fmt.Sprintf("%d", replicas)))

				cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				poolAllocatedBefore, _ = utils.Run(cmd)
			}, 2*time.Minute).Should(Succeed())

			By("recording Pool available count before restart")
			cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
				"-o", "jsonpath={.status.available}")
			poolAvailableBefore, _ := utils.Run(cmd)

			By("restarting the controller")
			err = restartController()
			Expect(err).NotTo(HaveOccurred())

			By("verifying Pool allocation state is reconstructed after restart")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				poolAllocatedAfter, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(poolAllocatedAfter).To(Equal(poolAllocatedBefore))

				cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				poolAvailableAfter, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(poolAvailableAfter).To(Equal(poolAvailableBefore))
			}, 30*time.Second).Should(Succeed())

			By("creating new BatchSandbox to verify no duplicate allocation occurs")
			const newBatchSandboxName = "test-bs-recovery-new"
			newBSYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": newBatchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         1,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			newBSFile := filepath.Join("/tmp", "test-bs-recovery-new.yaml")
			err = os.WriteFile(newBSFile, []byte(newBSYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(newBSFile)

			cmd = exec.Command("kubectl", "apply", "-f", newBSFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying new BatchSandbox gets allocated and Pool.allocated increases correctly")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", newBatchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))

				cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				poolAllocatedNew, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				before := 0
				if poolAllocatedBefore != "" {
					fmt.Sscanf(poolAllocatedBefore, "%d", &before)
				}
				after := 0
				if poolAllocatedNew != "" {
					fmt.Sscanf(poolAllocatedNew, "%d", &after)
				}
				g.Expect(after).To(Equal(before+1), "Pool.allocated should increase by 1")
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up resources")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "batchsandbox", newBatchSandboxName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})

		It("should reconstruct allocation for multiple batchsandboxes after restart", func() {
			const poolName = "test-pool-multi-bs"
			const bs1Name = "test-bs-1"
			const bs2Name = "test-bs-2"
			const testNamespace = "default"

			By("creating a Pool")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    5,
				"BufferMin":    3,
				"PoolMax":      10,
				"PoolMin":      5,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-multi.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).NotTo(BeEmpty())
			}, 2*time.Minute).Should(Succeed())

			By("creating two BatchSandboxes")
			bs1YAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": bs1Name,
				"Namespace":        testNamespace,
				"Replicas":         2,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())
			bs1File := filepath.Join("/tmp", "test-bs1.yaml")
			err = os.WriteFile(bs1File, []byte(bs1YAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bs1File)

			bs2YAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": bs2Name,
				"Namespace":        testNamespace,
				"Replicas":         3,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())
			bs2File := filepath.Join("/tmp", "test-bs2.yaml")
			err = os.WriteFile(bs2File, []byte(bs2YAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bs2File)

			cmd = exec.Command("kubectl", "apply", "-f", bs1File)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			cmd = exec.Command("kubectl", "apply", "-f", bs2File)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for allocations to complete")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", bs1Name, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output1, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output1).To(Equal("2"))

				cmd = exec.Command("kubectl", "get", "batchsandbox", bs2Name, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output2, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output2).To(Equal("3"))

				cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				allocatedStr, _ := utils.Run(cmd)
				allocated := 0
				fmt.Sscanf(allocatedStr, "%d", &allocated)
				g.Expect(allocated).To(Equal(5))
			}, 2*time.Minute).Should(Succeed())

			By("restarting the controller")
			err = restartController()
			Expect(err).NotTo(HaveOccurred())

			By("verifying Pool allocation state is correctly reconstructed")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				allocatedStr, _ := utils.Run(cmd)
				allocated := 0
				fmt.Sscanf(allocatedStr, "%d", &allocated)
				g.Expect(allocated).To(Equal(5))

				cmd = exec.Command("kubectl", "get", "batchsandbox", bs1Name, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output1, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output1).To(Equal("2"))

				cmd = exec.Command("kubectl", "get", "batchsandbox", bs2Name, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output2, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output2).To(Equal("3"))
			}, 30*time.Second).Should(Succeed())

			By("deleting first BatchSandbox and verifying only its pods are returned")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", bs1Name, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				allocatedStr, _ := utils.Run(cmd)
				allocated := 0
				fmt.Sscanf(allocatedStr, "%d", &allocated)
				g.Expect(allocated).To(Equal(3))
			}, 30*time.Second).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", bs2Name, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})

		It("should handle released pods correctly after controller restart", func() {
			const poolName = "test-pool-release"
			const batchSandboxName = "test-bs-release"
			const testNamespace = "default"

			By("creating a Pool")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    5,
				"BufferMin":    3,
				"PoolMax":      10,
				"PoolMin":      5,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-release.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				totalStr, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(totalStr).NotTo(BeEmpty())
			}, 2*time.Minute).Should(Succeed())

			By("creating a BatchSandbox with replicas=3")
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         3,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-release.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for allocation")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				allocatedStr, _ := utils.Run(cmd)
				allocated := 0
				fmt.Sscanf(allocatedStr, "%d", &allocated)
				g.Expect(allocated).To(Equal(3))
			}, 2*time.Minute).Should(Succeed())

			By("deleting BatchSandbox to release pods")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pods to be released")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				allocatedStr, _ := utils.Run(cmd)
				allocated := 0
				fmt.Sscanf(allocatedStr, "%d", &allocated)
				g.Expect(allocated).To(Equal(0))
			}, 30*time.Second).Should(Succeed())

			By("restarting the controller after pods are released")
			err = restartController()
			Expect(err).NotTo(HaveOccurred())

			By("verifying released pods are not re-allocated")
			Consistently(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				allocatedStr, _ := utils.Run(cmd)
				allocated := 0
				fmt.Sscanf(allocatedStr, "%d", &allocated)
				g.Expect(allocated).To(Equal(0))
			}, 10*time.Second, 2*time.Second).Should(Succeed())

			By("creating new BatchSandbox to verify it gets fresh pods")
			const newBatchSandboxName = "test-bs-release-new"
			newBSYAML, err := renderTemplate("testdata/batchsandbox-pooled-no-expire.yaml", map[string]interface{}{
				"BatchSandboxName": newBatchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         2,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			newBSFile := filepath.Join("/tmp", "test-bs-release-new.yaml")
			err = os.WriteFile(newBSFile, []byte(newBSYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(newBSFile)

			cmd = exec.Command("kubectl", "apply", "-f", newBSFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying new BatchSandbox gets allocated correctly")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", newBatchSandboxName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("2"))

				cmd = exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				allocatedStr, _ := utils.Run(cmd)
				allocated := 0
				fmt.Sscanf(allocatedStr, "%d", &allocated)
				g.Expect(allocated).To(Equal(2))
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", newBatchSandboxName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})

		It("should panic and exit when Recover fails due to malformed annotation", func() {
			const poolName = "test-pool-recover-fail"
			const batchSandboxName = "test-bs-malformed"
			const testNamespace = "default"

			By("first stopping controller to prevent early sync.Once execution")
			cmd := exec.Command("kubectl", "scale", "deployment", "opensandbox-controller-manager",
				"--replicas=0", "-n", namespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for controller pod to terminate")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
					"-n", namespace, "-o", "jsonpath={.items[*].metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(strings.TrimSpace(output)).To(BeEmpty())
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("creating a Pool (while controller is stopped)")
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": utils.SandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    3,
				"BufferMin":    2,
				"PoolMax":      5,
				"PoolMin":      2,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-recover-fail.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd = exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a BatchSandbox with malformed alloc-status annotation (while controller is stopped)")
			bsYAML, err := renderTemplate("testdata/batchsandbox-malformed-annotation.yaml", map[string]interface{}{
				"BatchSandboxName": batchSandboxName,
				"Namespace":        testNamespace,
				"Replicas":         1,
				"PoolName":         poolName,
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-bs-malformed.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying malformed annotation is set correctly")
			cmd = exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
				"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
			annoOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(annoOutput).To(ContainSubstring("invalid json"), "BatchSandbox should have malformed annotation")

			By("verifying BatchSandbox has poolRef set")
			cmd = exec.Command("kubectl", "get", "batchsandbox", batchSandboxName, "-n", testNamespace,
				"-o", "jsonpath={.spec.poolRef}")
			poolRefOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(poolRefOutput).To(Equal(poolName), "BatchSandbox should have poolRef set to %s", poolName)

			By("scaling up controller-manager - should exit due to malformed annotation during Recover")
			cmd = exec.Command("kubectl", "scale", "deployment", "opensandbox-controller-manager",
				"--replicas=1", "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for controller to process Pool, attempt Recover, and exit")
			// Wait long enough for:
			// 1. Container to start
			// 2. Leader election to complete
			// 3. Pool controller to reconcile and call Schedule -> checkRecovery -> Recover
			// 4. Recover to fail and call os.Exit(1)
			// 5. Container to restart
			time.Sleep(60 * time.Second)

			By("verifying container restarted and exited with code 1")
			// Get pod status to check restartCount and exit code
			cmd = exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
				"-n", namespace, "-o", "json")
			podJSON, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var podList struct {
				Items []struct {
					Status struct {
						ContainerStatuses []struct {
							RestartCount int `json:"restartCount"`
							LastState    struct {
								Terminated *struct {
									ExitCode int    `json:"exitCode"`
									Reason   string `json:"reason"`
								} `json:"terminated"`
							} `json:"lastState"`
							State struct {
								Waiting *struct {
									Reason  string `json:"reason"`
									Message string `json:"message"`
								} `json:"waiting"`
							} `json:"state"`
						} `json:"containerStatuses"`
					} `json:"status"`
				} `json:"items"`
			}
			err = json.Unmarshal([]byte(podJSON), &podList)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(podList.Items)).To(BeNumerically(">", 0), "Should have at least one controller pod")

			// Find a container that has restarted
			foundRestarted := false
			for _, pod := range podList.Items {
				for _, container := range pod.Status.ContainerStatuses {
					if container.RestartCount > 0 {
						foundRestarted = true
						// Verify the last termination had exit code 1
						if container.LastState.Terminated != nil {
							Expect(container.LastState.Terminated.ExitCode).To(Equal(1),
								"Container should exit with code 1 due to Recover failure")
						}
						break
					}
				}
				if foundRestarted {
					break
				}
			}
			Expect(foundRestarted).To(BeTrue(), "Container should have restarted due to Recover failure")

			By("cleaning up - deleting malformed BatchSandbox first")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", batchSandboxName, "-n", testNamespace)
			_, _ = utils.Run(cmd)

			By("restarting controller after cleanup - should succeed now")
			err = restartController()
			Expect(err).NotTo(HaveOccurred())

			By("cleaning up pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})
	})

})

// renderTemplate renders a YAML template file with the given data.
func renderTemplate(templateFile string, data map[string]interface{}) (string, error) {
	dir, err := utils.GetProjectDir()
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(dir, "test", "e2e", templateFile)
	tmplContent, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read template file %s: %w", fullPath, err)
	}

	tmpl, err := template.New("yaml").Parse(string(tmplContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// restartController scales down the controller-manager deployment to 0 and back to 1,
// simulating a controller restart to test state reconstruction.
func restartController() error {
	By("scaling down controller-manager to simulate restart")
	cmd := exec.Command("kubectl", "scale", "deployment", "opensandbox-controller-manager",
		"--replicas=0", "-n", namespace)
	_, err := utils.Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to scale down controller: %w", err)
	}

	By("waiting for controller pod to terminate")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
			"-n", namespace, "-o", "jsonpath={.items[*].metadata.name}")
		output, err := utils.Run(cmd)
		// Pod list should be empty when all pods are terminated
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(strings.TrimSpace(output)).To(BeEmpty())
	}, 30*time.Second, 2*time.Second).Should(Succeed())

	By("scaling up controller-manager")
	cmd = exec.Command("kubectl", "scale", "deployment", "opensandbox-controller-manager",
		"--replicas=1", "-n", namespace)
	_, err = utils.Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to scale up controller: %w", err)
	}

	By("waiting for controller to be ready after restart")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
			"-n", namespace, "-o", "jsonpath={.items[0].status.phase}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal("Running"))
	}, 2*time.Minute, 5*time.Second).Should(Succeed())

	return nil
}

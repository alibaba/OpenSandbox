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
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
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
				"SandboxImage": sandboxImage,
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
				"SandboxImage": sandboxImage,
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
				"SandboxImage": sandboxImage,
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
				"SandboxImage": sandboxImage,
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
				"SandboxImage": sandboxImage,
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
				"SandboxImage": sandboxImage,
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
				"SandboxImage":     sandboxImage,
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
			poolYAML, err := renderTemplate("testdata/pool-basic.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"SandboxImage": sandboxImage,
				"Namespace":    testNamespace,
				"BufferMax":    3,
				"BufferMin":    2,
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
				"SandboxImage":     sandboxImage,
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
				"SandboxImage":     sandboxImage,
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
				"SandboxImage": sandboxImage,
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
				"SandboxImage":     sandboxImage,
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
				"TaskExecutorImage": taskExecutorImage,
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
				"TaskExecutorImage": taskExecutorImage,
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

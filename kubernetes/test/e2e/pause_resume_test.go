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

const (
	pauseResumeNamespace = "default"
	registryServiceAddr  = "docker-registry.default.svc.cluster.local:5000"
	registryUsername     = "testuser"
	registryPassword     = "testpass"
)

var _ = Describe("PauseResume", Ordered, func() {
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		if err != nil {
			Expect(err.Error()).To(ContainSubstring("AlreadyExists"))
		}

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("kubectl", "apply", "-f", "config/crd/bases")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("kubectl", "apply", "-k", "config/default")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("waiting for controller to be ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
				"-n", namespace, "-o", "jsonpath={.items[0].status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Running"))
		}, 2*time.Minute).Should(Succeed())

		By("creating registry authentication secrets")
		err = createHtpasswdSecret(pauseResumeNamespace)
		Expect(err).NotTo(HaveOccurred())

		err = createDockerRegistrySecrets(pauseResumeNamespace)
		Expect(err).NotTo(HaveOccurred())

		By("deploying Docker Registry")
		registryYAML, err := renderTemplate("testdata/registry-deployment.yaml", nil)
		Expect(err).NotTo(HaveOccurred())

		registryFile := filepath.Join("/tmp", "test-registry.yaml")
		err = os.WriteFile(registryFile, []byte(registryYAML), 0644)
		Expect(err).NotTo(HaveOccurred())
		defer os.Remove(registryFile)

		cmd = exec.Command("kubectl", "apply", "-f", registryFile)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for registry to be ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment", "docker-registry",
				"-n", pauseResumeNamespace, "-o", "jsonpath={.status.availableReplicas}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("1"))
		}, 2*time.Minute).Should(Succeed())
	})

	AfterAll(func() {
		By("cleaning up Docker Registry")
		cmd := exec.Command("kubectl", "delete", "deployment", "docker-registry", "-n", pauseResumeNamespace, "--ignore-not-found=true")
		utils.Run(cmd)
		cmd = exec.Command("kubectl", "delete", "service", "docker-registry", "-n", pauseResumeNamespace, "--ignore-not-found=true")
		utils.Run(cmd)

		By("cleaning up secrets")
		for _, secret := range []string{"registry-auth", "registry-push-secret", "registry-pull-secret"} {
			cmd = exec.Command("kubectl", "delete", "secret", secret, "-n", pauseResumeNamespace, "--ignore-not-found=true")
			utils.Run(cmd)
		}

		By("cleaning up any remaining sandboxsnapshots")
		cmd = exec.Command("kubectl", "delete", "sandboxsnapshots", "--all", "-n", pauseResumeNamespace, "--ignore-not-found=true")
		utils.Run(cmd)

		By("cleaning up any remaining batchsandboxes")
		cmd = exec.Command("kubectl", "delete", "batchsandboxes", "--all", "-n", pauseResumeNamespace, "--ignore-not-found=true")
		utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("kubectl", "delete", "-k", "config/default", "--ignore-not-found=true")
		utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("kubectl", "delete", "-f", "config/crd/bases", "--ignore-not-found=true")
		utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found=true")
		utils.Run(cmd)
	})

	Context("Pause and Resume", func() {
		It("should complete the full pause-resume flow end-to-end", func() {
			const sandboxName = "test-pause-resume"
			const snapshotName = "test-pause-resume"

			// --- Step 1: Create BatchSandbox ---
			By("creating BatchSandbox with pausePolicy")
			bsYAML, err := renderTemplate("testdata/batchsandbox-with-pause-policy.yaml", map[string]interface{}{
				"BatchSandboxName":          sandboxName,
				"Namespace":                 pauseResumeNamespace,
				"SandboxImage":              utils.SandboxImage,
				"SnapshotRegistry":          registryServiceAddr,
				"SnapshotPushSecret":    "registry-push-secret",
				"ResumeImagePullSecret": "registry-pull-secret",
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-pause-resume-bs.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd := exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for BatchSandbox to be Running")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", sandboxName,
					"-n", pauseResumeNamespace, "-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 2*time.Minute).Should(Succeed())

			// --- Step 2: Get pod/node info ---
			By("getting pod and node info from BatchSandbox")
			cmd = exec.Command("kubectl", "get", "pods", "-n", pauseResumeNamespace, "-o", "json")
			podsJSON, err := utils.Run(cmd)
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
					Spec struct {
						NodeName string `json:"nodeName"`
					} `json:"spec"`
				} `json:"items"`
			}
			err = json.Unmarshal([]byte(podsJSON), &podList)
			Expect(err).NotTo(HaveOccurred())

			var podName, nodeName string
			for _, pod := range podList.Items {
				for _, owner := range pod.Metadata.OwnerReferences {
					if owner.Kind == "BatchSandbox" && owner.Name == sandboxName {
						podName = pod.Metadata.Name
						nodeName = pod.Spec.NodeName
						break
					}
				}
				if podName != "" {
					break
				}
			}
			Expect(podName).NotTo(BeEmpty(), "Should find a pod owned by BatchSandbox")

			// --- Step 2.5: Write marker file for rootfs verification ---
			markerValue := fmt.Sprintf("pause-test-%d", time.Now().UnixNano())
			By("writing marker file into container for rootfs verification")
			cmd = exec.Command("kubectl", "exec", podName, "-n", pauseResumeNamespace,
				"-c", "sandbox", "--", "sh", "-c", fmt.Sprintf("echo '%s' > /tmp/pause-marker", markerValue))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// --- Step 3: Create SandboxSnapshot ---
			By("creating SandboxSnapshot CR")
			pausedAt := time.Now().UTC().Format(time.RFC3339)
			snapshotYAML, err := renderTemplate("testdata/sandboxsnapshot.yaml", map[string]interface{}{
				"SnapshotName":              snapshotName,
				"Namespace":                 pauseResumeNamespace,
				"SandboxId":                 sandboxName,
				"SourceBatchSandboxName":    sandboxName,
				"SourcePodName":             podName,
				"SourceNodeName":            nodeName,
				"SnapshotRegistry":          registryServiceAddr,
				"ImageUri":                  fmt.Sprintf("%s/%s:snapshot", registryServiceAddr, sandboxName),
				"SnapshotPushSecret":    "registry-push-secret",
				"ResumeImagePullSecret": "registry-pull-secret",
				"SandboxImage":              utils.SandboxImage,
				"PausedAt":                  pausedAt,
			})
			Expect(err).NotTo(HaveOccurred())

			snapshotFile := filepath.Join("/tmp", "test-pause-resume-snapshot.yaml")
			err = os.WriteFile(snapshotFile, []byte(snapshotYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(snapshotFile)

			cmd = exec.Command("kubectl", "apply", "-f", snapshotFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// --- Step 4: Wait for snapshot Ready ---
			By("waiting for SandboxSnapshot to be Ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "sandboxsnapshot", snapshotName,
					"-n", pauseResumeNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ready"))
			}, 3*time.Minute).Should(Succeed())

			// --- Step 5: Verify commit Job succeeded ---
			By("verifying commit Job completed successfully")
			cmd = exec.Command("kubectl", "get", "job", fmt.Sprintf("%s-commit-v1", snapshotName),
				"-n", pauseResumeNamespace, "-o", "jsonpath={.status.succeeded}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("1"))

			// --- Step 6: Verify status.containerSnapshots populated ---
			By("verifying snapshot status has containerSnapshots with imageUri")
			cmd = exec.Command("kubectl", "get", "sandboxsnapshot", snapshotName,
				"-n", pauseResumeNamespace, "-o", "jsonpath={.status.containerSnapshots[0].imageUri}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "Snapshot status should contain containerSnapshots with imageUri")

			// --- Step 7: Verify source BatchSandbox was auto-deleted by handleReady ---
			By("verifying source BatchSandbox was auto-deleted after snapshot Ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", sandboxName, "-n", pauseResumeNamespace)
				output, err := utils.Run(cmd)
				g.Expect(output).To(ContainSubstring("NotFound"))
				g.Expect(err).To(HaveOccurred())
			}, 30*time.Second).Should(Succeed())

			// --- Step 8: Resume - patch Snapshot CR to trigger controller resume ---
			By("patching SandboxSnapshot resumeVersion to trigger resume")
			cmd = exec.Command("kubectl", "patch", "sandboxsnapshot", snapshotName,
				"-n", pauseResumeNamespace, "--type=merge",
				"-p", `{"spec":{"resumeVersion":1}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for controller to ACK resumeVersion")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "sandboxsnapshot", snapshotName,
					"-n", pauseResumeNamespace, "-o", "jsonpath={.status.resumeVersion}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 30*time.Second).Should(Succeed())

			By("waiting for resumed BatchSandbox to be Running")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", sandboxName,
					"-n", pauseResumeNamespace, "-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 2*time.Minute).Should(Succeed())

			// --- Step 8.5: Verify rootfs data persistence ---
			By("getting resumed pod name")
			cmd = exec.Command("kubectl", "get", "pods", "-n", pauseResumeNamespace, "-o", "json")
			resumedPodsJSON, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var resumedPodList struct {
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
			err = json.Unmarshal([]byte(resumedPodsJSON), &resumedPodList)
			Expect(err).NotTo(HaveOccurred())

			var resumedPodName string
			for _, pod := range resumedPodList.Items {
				for _, owner := range pod.Metadata.OwnerReferences {
					if owner.Kind == "BatchSandbox" && owner.Name == sandboxName {
						resumedPodName = pod.Metadata.Name
						break
					}
				}
				if resumedPodName != "" {
					break
				}
			}
			Expect(resumedPodName).NotTo(BeEmpty(), "Should find a pod owned by resumed BatchSandbox")

			By("reading marker file from resumed container to verify rootfs persistence")
			cmd = exec.Command("kubectl", "exec", resumedPodName, "-n", pauseResumeNamespace,
				"-c", "sandbox", "--", "cat", "/tmp/pause-marker")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(output)).To(Equal(markerValue),
				"Rootfs data should persist across pause/resume")

			By("verifying resumed-from-snapshot annotation on BatchSandbox")
			cmd = exec.Command("kubectl", "get", "batchsandbox", sandboxName,
				"-n", pauseResumeNamespace, "-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/resumed-from-snapshot}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("true"))

			By("verifying snapshot history has Pause and Resume records")
			cmd = exec.Command("kubectl", "get", "sandboxsnapshot", snapshotName,
				"-n", pauseResumeNamespace, "-o", "jsonpath={.status.history[*].action}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Pause"))
			Expect(output).To(ContainSubstring("Resume"))

			// --- Cleanup ---
			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", sandboxName, "-n", pauseResumeNamespace)
			utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "sandboxsnapshot", snapshotName, "-n", pauseResumeNamespace)
			utils.Run(cmd)
		})

		It("should complete pool-based pause-resume with rootfs verification", func() {
			const poolName = "test-pool-pause"
			const sandboxName = "test-pool-pause-resume"
			const snapshotName = "test-pool-pause-snap"

			// --- Step 1: Create Pool CR ---
			By("creating Pool CR")
			poolYAML, err := renderTemplate("testdata/pool-with-pause-policy.yaml", map[string]interface{}{
				"PoolName":     poolName,
				"Namespace":    pauseResumeNamespace,
				"SandboxImage": utils.SandboxImage,
				"BufferMax":    2,
				"BufferMin":    1,
				"PoolMax":      5,
				"PoolMin":      1,
			})
			Expect(err).NotTo(HaveOccurred())

			poolFile := filepath.Join("/tmp", "test-pool-pause.yaml")
			err = os.WriteFile(poolFile, []byte(poolYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(poolFile)

			cmd := exec.Command("kubectl", "apply", "-f", poolFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Pool to have available pods")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName,
					"-n", pauseResumeNamespace, "-o", "jsonpath={.status.available}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty())
				g.Expect(output).NotTo(Equal("0"))
			}, 2*time.Minute).Should(Succeed())

			// --- Step 2: Create BatchSandbox with poolRef + pausePolicy ---
			By("creating BatchSandbox with poolRef and pausePolicy")
			bsYAML, err := renderTemplate("testdata/batchsandbox-pooled-pause.yaml", map[string]interface{}{
				"BatchSandboxName":          sandboxName,
				"Namespace":                 pauseResumeNamespace,
				"PoolName":                  poolName,
				"SnapshotRegistry":          registryServiceAddr,
				"SnapshotPushSecret":    "registry-push-secret",
				"ResumeImagePullSecret": "registry-pull-secret",
			})
			Expect(err).NotTo(HaveOccurred())

			bsFile := filepath.Join("/tmp", "test-pool-pause-bs.yaml")
			err = os.WriteFile(bsFile, []byte(bsYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(bsFile)

			cmd = exec.Command("kubectl", "apply", "-f", bsFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for BatchSandbox to be Running")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", sandboxName,
					"-n", pauseResumeNamespace, "-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 2*time.Minute).Should(Succeed())

			// --- Step 3: Get pod name from alloc-status ---
			By("getting pod name from alloc-status annotation")
			var podName string
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", sandboxName,
					"-n", pauseResumeNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				allocStatusJSON, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(allocStatusJSON).NotTo(BeEmpty(), "alloc-status annotation should exist")

				var allocStatus struct {
					Pods []string `json:"pods"`
				}
				err = json.Unmarshal([]byte(allocStatusJSON), &allocStatus)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(allocStatus.Pods)).To(BeNumerically(">=", 1))
				podName = allocStatus.Pods[0]
			}).Should(Succeed())
			Expect(podName).NotTo(BeEmpty(), "Should have allocated pod name")

			// --- Step 4: Write marker file ---
			markerValue := fmt.Sprintf("pool-pause-test-%d", time.Now().UnixNano())
			By("writing marker file into container for rootfs verification")
			cmd = exec.Command("kubectl", "exec", podName, "-n", pauseResumeNamespace,
				"-c", "sandbox", "--", "sh", "-c", fmt.Sprintf("echo '%s' > /tmp/pause-marker", markerValue))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// --- Step 5: Create minimal SandboxSnapshot (controller resolves via poolRef) ---
			By("creating minimal SandboxSnapshot CR (controller resolves template from Pool CR)")
			pausedAt := time.Now().UTC().Format(time.RFC3339)
			snapshotYAML, err := renderTemplate("testdata/sandboxsnapshot-minimal.yaml", map[string]interface{}{
				"SnapshotName":           snapshotName,
				"Namespace":              pauseResumeNamespace,
				"SandboxId":              sandboxName,
				"SourceBatchSandboxName": sandboxName,
				"PausedAt":               pausedAt,
			})
			Expect(err).NotTo(HaveOccurred())

			snapshotFile := filepath.Join("/tmp", "test-pool-pause-snapshot.yaml")
			err = os.WriteFile(snapshotFile, []byte(snapshotYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(snapshotFile)

			cmd = exec.Command("kubectl", "apply", "-f", snapshotFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// --- Step 6: Wait for snapshot Ready ---
			By("waiting for SandboxSnapshot to be Ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "sandboxsnapshot", snapshotName,
					"-n", pauseResumeNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Ready"))
			}, 3*time.Minute).Should(Succeed())

			// --- Step 7: Verify source BatchSandbox was auto-deleted ---
			By("verifying source BatchSandbox was auto-deleted after snapshot Ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", sandboxName, "-n", pauseResumeNamespace)
				output, err := utils.Run(cmd)
				g.Expect(output).To(ContainSubstring("NotFound"))
				g.Expect(err).To(HaveOccurred())
			}, 30*time.Second).Should(Succeed())

			// --- Step 8: Resume ---
			By("patching SandboxSnapshot resumeVersion to trigger resume")
			cmd = exec.Command("kubectl", "patch", "sandboxsnapshot", snapshotName,
				"-n", pauseResumeNamespace, "--type=merge",
				"-p", `{"spec":{"resumeVersion":1}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for controller to ACK resumeVersion")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "sandboxsnapshot", snapshotName,
					"-n", pauseResumeNamespace, "-o", "jsonpath={.status.resumeVersion}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 30*time.Second).Should(Succeed())

			By("waiting for resumed BatchSandbox to be Running")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", sandboxName,
					"-n", pauseResumeNamespace, "-o", "jsonpath={.status.ready}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}, 2*time.Minute).Should(Succeed())

			// --- Step 9: Verify rootfs data persistence ---
			By("getting resumed pod name")
			cmd = exec.Command("kubectl", "get", "pods", "-n", pauseResumeNamespace, "-o", "json")
			resumedPodsJSON, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var resumedPodList struct {
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
			err = json.Unmarshal([]byte(resumedPodsJSON), &resumedPodList)
			Expect(err).NotTo(HaveOccurred())

			var resumedPodName string
			for _, pod := range resumedPodList.Items {
				for _, owner := range pod.Metadata.OwnerReferences {
					if owner.Kind == "BatchSandbox" && owner.Name == sandboxName {
						resumedPodName = pod.Metadata.Name
						break
					}
				}
				if resumedPodName != "" {
					break
				}
			}
			Expect(resumedPodName).NotTo(BeEmpty(), "Should find a pod owned by resumed BatchSandbox")

			By("reading marker file from resumed container to verify rootfs persistence")
			cmd = exec.Command("kubectl", "exec", resumedPodName, "-n", pauseResumeNamespace,
				"-c", "sandbox", "--", "cat", "/tmp/pause-marker")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(output)).To(Equal(markerValue),
				"Rootfs data should persist across pause/resume")

			// --- Step 10: Verify history records ---
			By("verifying snapshot history has Pause and Resume records")
			cmd = exec.Command("kubectl", "get", "sandboxsnapshot", snapshotName,
				"-n", pauseResumeNamespace, "-o", "jsonpath={.status.history[*].action}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Pause"))
			Expect(output).To(ContainSubstring("Resume"))

			// --- Cleanup ---
			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", sandboxName, "-n", pauseResumeNamespace, "--ignore-not-found=true")
			utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "sandboxsnapshot", snapshotName, "-n", pauseResumeNamespace, "--ignore-not-found=true")
			utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", pauseResumeNamespace, "--ignore-not-found=true")
			utils.Run(cmd)
		})
	})

	Context("Failure", func() {
		It("should transition to Failed when source Pod does not exist", func() {
			const snapshotName = "test-pause-fail"

			By("creating SandboxSnapshot with non-existent source")
			pausedAt := time.Now().UTC().Format(time.RFC3339)
			snapshotYAML, err := renderTemplate("testdata/sandboxsnapshot.yaml", map[string]interface{}{
				"SnapshotName":              snapshotName,
				"Namespace":                 pauseResumeNamespace,
				"SandboxId":                 "nonexistent-sandbox",
				"SourceBatchSandboxName":    "nonexistent-sandbox",
				"SourcePodName":             "nonexistent-pod",
				"SourceNodeName":            "nonexistent-node",
				"SnapshotRegistry":          registryServiceAddr,
				"ImageUri":                  fmt.Sprintf("%s/nonexistent:snapshot", registryServiceAddr),
				"SnapshotPushSecret":    "registry-push-secret",
				"ResumeImagePullSecret": "registry-pull-secret",
				"SandboxImage":              utils.SandboxImage,
				"PausedAt":                  pausedAt,
			})
			Expect(err).NotTo(HaveOccurred())

			snapshotFile := filepath.Join("/tmp", "test-pause-fail-snapshot.yaml")
			err = os.WriteFile(snapshotFile, []byte(snapshotYAML), 0644)
			Expect(err).NotTo(HaveOccurred())
			defer os.Remove(snapshotFile)

			cmd := exec.Command("kubectl", "apply", "-f", snapshotFile)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for SandboxSnapshot to reach Failed phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "sandboxsnapshot", snapshotName,
					"-n", pauseResumeNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Failed"))
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "sandboxsnapshot", snapshotName, "-n", pauseResumeNamespace)
			utils.Run(cmd)
		})
	})
})

// createHtpasswdSecret creates the htpasswd secret for registry authentication.
// Docker Registry v2 only supports bcrypt hashes, not MD5 ($apr1$) or SHA1.
func createHtpasswdSecret(namespace string) error {
	htpasswdEntry := ""
	pyCmd := exec.Command("python3", "-c",
		fmt.Sprintf("import bcrypt; print('%s:' + bcrypt.hashpw(b'%s', bcrypt.gensalt(rounds=10)).decode())",
			registryUsername, registryPassword))
	if output, err := pyCmd.Output(); err == nil {
		htpasswdEntry = strings.TrimSpace(string(output))
	}

	if htpasswdEntry == "" {
		return fmt.Errorf("failed to generate bcrypt htpasswd: python3 bcrypt not available")
	}

	tmpFile := filepath.Join(os.TempDir(), "htpasswd")
	if err := os.WriteFile(tmpFile, []byte(htpasswdEntry), 0644); err != nil {
		return fmt.Errorf("failed to write htpasswd file: %w", err)
	}
	defer os.Remove(tmpFile)

	cmd := exec.Command("kubectl", "create", "secret", "generic", "registry-auth",
		"--from-file=htpasswd="+tmpFile, "-n", namespace)
	if _, err := utils.Run(cmd); err != nil {
		cmd = exec.Command("kubectl", "delete", "secret", "registry-auth", "-n", namespace, "--ignore-not-found=true")
		utils.Run(cmd)
		cmd = exec.Command("kubectl", "create", "secret", "generic", "registry-auth",
			"--from-file=htpasswd="+tmpFile, "-n", namespace)
		if _, err := utils.Run(cmd); err != nil {
			return fmt.Errorf("failed to create registry-auth secret: %w", err)
		}
	}

	return nil
}

// createDockerRegistrySecrets creates docker-registry secrets for push/pull.
func createDockerRegistrySecrets(namespace string) error {
	server := registryServiceAddr

	cmd := exec.Command("kubectl", "create", "secret", "docker-registry", "registry-push-secret",
		"--docker-server="+server,
		"--docker-username="+registryUsername,
		"--docker-password="+registryPassword,
		"-n", namespace)
	if _, err := utils.Run(cmd); err != nil {
		cmd = exec.Command("kubectl", "delete", "secret", "registry-push-secret", "-n", namespace, "--ignore-not-found=true")
		utils.Run(cmd)
		cmd = exec.Command("kubectl", "create", "secret", "docker-registry", "registry-push-secret",
			"--docker-server="+server,
			"--docker-username="+registryUsername,
			"--docker-password="+registryPassword,
			"-n", namespace)
		if _, err := utils.Run(cmd); err != nil {
			return fmt.Errorf("failed to create registry-push-secret: %w", err)
		}
	}

	cmd = exec.Command("kubectl", "create", "secret", "docker-registry", "registry-pull-secret",
		"--docker-server="+server,
		"--docker-username="+registryUsername,
		"--docker-password="+registryPassword,
		"-n", namespace)
	if _, err := utils.Run(cmd); err != nil {
		cmd = exec.Command("kubectl", "delete", "secret", "registry-pull-secret", "-n", namespace, "--ignore-not-found=true")
		utils.Run(cmd)
		cmd = exec.Command("kubectl", "create", "secret", "docker-registry", "registry-pull-secret",
			"--docker-server="+server,
			"--docker-username="+registryUsername,
			"--docker-password="+registryPassword,
			"-n", namespace)
		if _, err := utils.Run(cmd); err != nil {
			return fmt.Errorf("failed to create registry-pull-secret: %w", err)
		}
	}

	return nil
}

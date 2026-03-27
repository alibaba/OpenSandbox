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
	"os/exec"
	"strings"
	"time"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/internal/controller"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/test/utils"
)

// Pod Recycle Policy E2E Tests
var _ = Describe("Pod Recycle Policy", Ordered, func() {
	const testNamespace = "default"

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

		By("patching controller deployment with restart-timeout for testing")
		cmd = exec.Command("kubectl", "patch", "deployment", "opensandbox-controller-manager", "-n", namespace,
			"--type", "json", "-p",
			`[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--restart-timeout=60s"}]`)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch controller deployment")

		By("waiting for controller rollout to complete")
		cmd = exec.Command("kubectl", "rollout", "status", "deployment/opensandbox-controller-manager", "-n", namespace, "--timeout=60s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to wait for controller rollout")

		By("waiting for controller to be ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
				"-n", namespace, "-o", "jsonpath={.items[0].status.phase}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("Running"))
		}, 2*time.Minute).Should(Succeed())
	})

	AfterAll(func() {
		By("undeploying the controller-manager")
		cmd := exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(2 * time.Second)

	Context("Delete Policy", func() {
		It("should delete pod when BatchSandbox is deleted with Delete policy", func() {
			poolName := "delete-policy-pool"
			bsbxName := "delete-policy-bsbx"

			By("creating Pool with Delete policy")
			poolYAML := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: Pool
metadata:
  name: %s
  namespace: %s
spec:
  podRecyclePolicy: Delete
  template:
    spec:
      containers:
      - name: sandbox-container
        image: task-executor:dev
        command: ["/bin/sh", "-c", "trap 'exit 0' TERM; while true; do sleep 1; done"]
  capacitySpec:
    bufferMax: 1
    bufferMin: 1
    poolMax: 1
    poolMin: 1
`, poolName, testNamespace)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(poolYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("waiting for Pool to have available pods")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}).Should(Succeed())

			By("creating BatchSandbox")
			bsbxYAML := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: BatchSandbox
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 1
  poolRef: %s
`, bsbxName, testNamespace, poolName)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(bsbxYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create BatchSandbox")

			By("waiting for BatchSandbox to be allocated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", bsbxName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}).Should(Succeed())

			By("getting the allocated pod name")
			cmd = exec.Command("kubectl", "get", "batchsandbox", bsbxName, "-n", testNamespace,
				"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			var alloc controller.SandboxAllocation
			Expect(json.Unmarshal([]byte(output), &alloc)).To(Succeed())
			Expect(alloc.Pods).To(HaveLen(1))
			podName := alloc.Pods[0]

			By("deleting BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", bsbxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete BatchSandbox")

			By("verifying pod is deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace, "--ignore-not-found")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(BeEmpty(), "Pod should be deleted with Delete policy")
			}).Should(Succeed())

			By("cleaning up Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace, "--timeout=30s")
			_, _ = utils.Run(cmd)
		})
	})

	Context("Restart Policy - Success", func() {
		It("should restart and reuse pod when BatchSandbox is deleted with Restart policy", func() {
			poolName := "restart-policy-pool"
			bsbxName := "restart-policy-bsbx"

			By("creating Pool with Restart policy")
			poolYAML := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: Pool
metadata:
  name: %s
  namespace: %s
spec:
  podRecyclePolicy: Restart
  template:
    spec:
      containers:
      - name: sandbox-container
        image: task-executor:dev
        command: ["/bin/sh", "-c", "trap 'exit 0' TERM; while true; do sleep 1; done"]
  capacitySpec:
    bufferMax: 1
    bufferMin: 1
    poolMax: 1
    poolMin: 1
`, poolName, testNamespace)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(poolYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("waiting for Pool to have available pods")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}).Should(Succeed())

			By("creating BatchSandbox")
			bsbxYAML := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: BatchSandbox
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 1
  poolRef: %s
`, bsbxName, testNamespace, poolName)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(bsbxYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create BatchSandbox")

			By("waiting for BatchSandbox to be allocated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", bsbxName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}).Should(Succeed())

			By("getting the allocated pod name and initial restart count")
			cmd = exec.Command("kubectl", "get", "batchsandbox", bsbxName, "-n", testNamespace,
				"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			var alloc controller.SandboxAllocation
			Expect(json.Unmarshal([]byte(output), &alloc)).To(Succeed())
			Expect(alloc.Pods).To(HaveLen(1))
			podName := alloc.Pods[0]

			cmd = exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
				"-o", "jsonpath={.status.containerStatuses[0].restartCount}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			initialRestartCount := output

			By("deleting BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", bsbxName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete BatchSandbox")

			By("verifying pod is NOT deleted")
			Consistently(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace, "--ignore-not-found", "-o", "name")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring(podName), "Pod should NOT be deleted with Restart policy")
			}, 30*time.Second).Should(Succeed())

			By("waiting for pod restart count to increase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
					"-o", "jsonpath={.status.containerStatuses[0].restartCount}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).ToNot(Equal(initialRestartCount), "Restart count should increase")
			}).Should(Succeed())

			By("waiting for recycle-meta annotation to be cleared (restart completed)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/recycle-meta}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(BeEmpty(), "recycle-meta annotation should be cleared after restart completes")
			}).Should(Succeed())

			By("waiting for pod to be Ready again")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Pod should be Ready after restart")
			}).Should(Succeed())

			By("verifying pod is available for reuse (deallocated-from label cleared)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.labels.pool\\.opensandbox\\.io/deallocated-from}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(BeEmpty(), "deallocated-from label should be cleared for reuse")
			}).Should(Succeed())

			By("creating new BatchSandbox to verify pod can be reused")
			bsbxName2 := "restart-policy-bsbx-2"
			bsbxYAML2 := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: BatchSandbox
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 1
  poolRef: %s
`, bsbxName2, testNamespace, poolName)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			GinkgoWriter.Printf("Creating second BatchSandbox %s\n", bsbxYAML2)
			cmd.Stdin = strings.NewReader(bsbxYAML2)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create second BatchSandbox")

			By("verifying the same pod is reused")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", bsbxName2, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				var alloc2 controller.SandboxAllocation
				g.Expect(json.Unmarshal([]byte(output), &alloc2)).To(Succeed())
				g.Expect(alloc2.Pods).To(ContainElement(podName), "Same pod should be reused")
			}).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", bsbxName2, "-n", testNamespace, "--timeout=60s")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace, "--timeout=30s")
			_, _ = utils.Run(cmd)
		})
	})

	Context("Restart Policy - Failure", func() {
		It("should delete pod when restart times out", func() {
			poolName := "restart-timeout-pool"
			bsbxName := "restart-timeout-bsbx"

			By("creating Pool with Restart policy and a container that exits immediately")
			poolYAML := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: Pool
metadata:
  name: %s
  namespace: %s
spec:
  podRecyclePolicy: Restart
  template:
    spec:
      containers:
      - name: sandbox-container
        image: task-executor:dev
        command: ["/bin/sh", "-c", "sleep infinity"]
  capacitySpec:
    bufferMax: 1
    bufferMin: 1
    poolMax: 1
    poolMin: 1
`, poolName, testNamespace)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(poolYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("waiting for Pool to have pods created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.total}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}).Should(Succeed())

			By("creating BatchSandbox")
			bsbxYAML := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: BatchSandbox
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 1
  poolRef: %s
`, bsbxName, testNamespace, poolName)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(bsbxYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create BatchSandbox")

			By("getting the pod name")
			time.Sleep(3 * time.Second)
			cmd = exec.Command("kubectl", "get", "pods", "-n", testNamespace,
				"-l", "sandbox.opensandbox.io/pool-name="+poolName,
				"-o", "jsonpath={.items[0].metadata.name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			podName := output
			Expect(podName).NotTo(BeEmpty())

			By("deleting BatchSandbox to trigger restart")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", bsbxName, "-n", testNamespace, "--timeout=60s")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete BatchSandbox")

			By("waiting for restart timeout - pod should be marked for deletion or already deleted")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.deletionTimestamp}")
				output, err := utils.Run(cmd)
				success := (err == nil && output != "") || (err != nil && strings.Contains(err.Error(), "not found"))
				g.Expect(success).To(BeTrue(), "Pod %s should have deletionTimestamp or be deleted", podName)
			}, 60*time.Second).Should(Succeed())

			By("cleaning up Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace, "--timeout=30s")
			_, _ = utils.Run(cmd)
		})
	})

	Context("Batch Operations", func() {
		It("should handle multiple BatchSandbox deletions with Restart policy", func() {
			poolName := "batch-ops-pool"

			By("creating Pool with Restart policy")
			poolYAML := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: Pool
metadata:
  name: %s
  namespace: %s
spec:
  podRecyclePolicy: Restart
  template:
    spec:
      containers:
      - name: sandbox-container
        image: task-executor:dev
        command: ["/bin/sh", "-c", "trap 'exit 0' TERM; while true; do sleep 1; done"]
  capacitySpec:
    bufferMax: 0
    bufferMin: 0
    poolMax: 3
    poolMin: 3
`, poolName, testNamespace)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(poolYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("waiting for Pool to have available pods")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("3"))
			}).Should(Succeed())

			By("creating multiple BatchSandboxes")
			bsbxNames := []string{"batch-ops-bsbx-1", "batch-ops-bsbx-2", "batch-ops-bsbx-3"}
			for _, bsbxName := range bsbxNames {
				bsbxYAML := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: BatchSandbox
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 1
  poolRef: %s
`, bsbxName, testNamespace, poolName)
				cmd := exec.Command("kubectl", "apply", "-f", "-")
				cmd.Stdin = strings.NewReader(bsbxYAML)
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to create BatchSandbox "+bsbxName)
			}

			By("waiting for all BatchSandboxes to be allocated")
			for _, bsbxName := range bsbxNames {
				Eventually(func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "batchsandbox", bsbxName, "-n", testNamespace,
						"-o", "jsonpath={.status.allocated}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal("1"))
				}).Should(Succeed())
			}

			By("recording pod names before deletion")
			podNames := make([]string, 0)
			for _, bsbxName := range bsbxNames {
				cmd := exec.Command("kubectl", "get", "batchsandbox", bsbxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.annotations.sandbox\\.opensandbox\\.io/alloc-status}")
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
				var alloc controller.SandboxAllocation
				Expect(json.Unmarshal([]byte(output), &alloc)).To(Succeed())
				podNames = append(podNames, alloc.Pods...)
			}
			Expect(podNames).To(HaveLen(3))

			By("deleting all BatchSandboxes")
			for _, bsbxName := range bsbxNames {
				cmd := exec.Command("kubectl", "delete", "batchsandbox", bsbxName, "-n", testNamespace, "--timeout=60s")
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Failed to delete BatchSandbox "+bsbxName)
			}

			By("waiting for all pods to complete restart and be available")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("3"), "All pods should be available after restart")
			}).Should(Succeed())

			By("verifying all original pods are still present (not deleted)")
			for _, podName := range podNames {
				cmd := exec.Command("kubectl", "get", "pod", podName, "-n", testNamespace, "--ignore-not-found", "-o", "name")
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(ContainSubstring(podName), "Pod %s should still exist", podName)
			}

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace, "--timeout=30s")
			_, _ = utils.Run(cmd)
		})
	})

	Context("Pool Recycle Finalizer", func() {
		It("should block BatchSandbox deletion until pods are recycled", func() {
			poolName := "finalizer-pool"
			bsbxName := "finalizer-bsbx"

			By("creating Pool with Restart policy")
			poolYAML := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: Pool
metadata:
  name: %s
  namespace: %s
spec:
  podRecyclePolicy: Restart
  template:
    spec:
      containers:
      - name: sandbox-container
        image: task-executor:dev
        command: ["/bin/sh", "-c", "trap 'exit 0' TERM; while true; do sleep 1; done"]
  capacitySpec:
    bufferMax: 1
    bufferMin: 1
    poolMax: 1
    poolMin: 1
`, poolName, testNamespace)
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(poolYAML)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pool")

			By("waiting for Pool to have available pods")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pool", poolName, "-n", testNamespace,
					"-o", "jsonpath={.status.available}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}).Should(Succeed())

			By("creating BatchSandbox")
			bsbxYAML := fmt.Sprintf(`
apiVersion: sandbox.opensandbox.io/v1alpha1
kind: BatchSandbox
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 1
  poolRef: %s
`, bsbxName, testNamespace, poolName)
			cmd = exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(bsbxYAML)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create BatchSandbox")

			By("waiting for BatchSandbox to be allocated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", bsbxName, "-n", testNamespace,
					"-o", "jsonpath={.status.allocated}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"))
			}).Should(Succeed())

			By("verifying pool-recycle finalizer is present")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", bsbxName, "-n", testNamespace,
					"-o", "jsonpath={.metadata.finalizers}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("batch-sandbox.sandbox.opensandbox.io/pool-recycle"))
			}).Should(Succeed())

			By("deleting BatchSandbox")
			cmd = exec.Command("kubectl", "delete", "batchsandbox", bsbxName, "-n", testNamespace, "--timeout=60s")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete BatchSandbox")

			By("verifying BatchSandbox is deleted (finalizer removed after recycle)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "batchsandbox", bsbxName, "-n", testNamespace, "--ignore-not-found")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(BeEmpty(), "BatchSandbox should be deleted after finalizer is removed")
			}).Should(Succeed())

			By("cleaning up Pool")
			cmd = exec.Command("kubectl", "delete", "pool", poolName, "-n", testNamespace, "--timeout=30s")
			_, _ = utils.Run(cmd)
		})
	})
})

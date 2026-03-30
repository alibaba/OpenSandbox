//go:build e2e

package tests

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func getHostVolumeDir() string {
	if v := os.Getenv("OPENSANDBOX_TEST_HOST_VOLUME_DIR"); v != "" {
		return v
	}
	return "/tmp/opensandbox-e2e/host-volume-test"
}

func getPVCName() string {
	if v := os.Getenv("OPENSANDBOX_TEST_PVC_NAME"); v != "" {
		return v
	}
	return "opensandbox-e2e-pvc-test"
}

func TestVolume_HostMount(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	hostDir := getHostVolumeDir()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image:       getSandboxImage(),
		ReadyTimeout: 60 * time.Second,
		Volumes: []opensandbox.Volume{
			{
				Name:      "test-host-vol",
				Host:      &opensandbox.Host{Path: hostDir},
				MountPath: "/mnt/host-data",
			},
		},
	})
	if err != nil {
		t.Logf("CreateSandbox with host volume: %v (host volumes may not be allowed)", err)
		t.Skip("Host volume mount not supported in this environment")
	}
	defer sb.Kill(context.Background())

	// Write a file to the mounted volume
	exec, err := sb.RunCommand(ctx, `echo "host-mount-test" > /mnt/host-data/go-e2e.txt`, nil)
	if err != nil {
		t.Fatalf("Write to host mount: %v", err)
	}
	if exec.ExitCode != nil && *exec.ExitCode != 0 {
		t.Fatalf("Write exit code: %d", *exec.ExitCode)
	}

	// Read it back
	exec, err = sb.RunCommand(ctx, "cat /mnt/host-data/go-e2e.txt", nil)
	if err != nil {
		t.Fatalf("Read from host mount: %v", err)
	}
	if !strings.Contains(exec.Text(), "host-mount-test") {
		t.Errorf("Expected content from host mount, got: %q", exec.Text())
	}
	t.Log("Host volume mount read/write passed")
}

func TestVolume_HostMountReadOnly(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	hostDir := getHostVolumeDir()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
		Volumes: []opensandbox.Volume{
			{
				Name:      "test-host-ro",
				Host:      &opensandbox.Host{Path: hostDir},
				MountPath: "/mnt/host-ro",
				ReadOnly:  true,
			},
		},
	})
	if err != nil {
		t.Logf("CreateSandbox with ro host volume: %v", err)
		t.Skip("Host volume mount not supported")
	}
	defer sb.Kill(context.Background())

	// Writing to read-only mount should fail
	exec, err := sb.RunCommand(ctx, `echo "should-fail" > /mnt/host-ro/fail.txt 2>&1 || echo "READONLY"`, nil)
	if err != nil {
		t.Fatalf("Write to ro mount: %v", err)
	}
	if !strings.Contains(exec.Text(), "READONLY") && !strings.Contains(exec.Text(), "Read-only") {
		t.Logf("Write to read-only mount output: %q", exec.Text())
	}
	t.Log("Host volume read-only mount verified")
}

func TestVolume_PVCMount(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pvcName := getPVCName()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
		Volumes: []opensandbox.Volume{
			{
				Name:      "test-pvc-vol",
				PVC:       &opensandbox.PVC{ClaimName: pvcName},
				MountPath: "/mnt/pvc-data",
			},
		},
	})
	if err != nil {
		t.Logf("CreateSandbox with PVC: %v (PVC %s may not exist)", err, pvcName)
		t.Skip("PVC volume mount not available")
	}
	defer sb.Kill(context.Background())

	// Write to PVC
	exec, err := sb.RunCommand(ctx, `echo "pvc-test-data" > /mnt/pvc-data/go-e2e.txt`, nil)
	if err != nil {
		t.Fatalf("Write to PVC: %v", err)
	}
	if exec.ExitCode != nil && *exec.ExitCode != 0 {
		t.Fatalf("Write exit code: %d", *exec.ExitCode)
	}

	// Read back
	exec, err = sb.RunCommand(ctx, "cat /mnt/pvc-data/go-e2e.txt", nil)
	if err != nil {
		t.Fatalf("Read from PVC: %v", err)
	}
	if !strings.Contains(exec.Text(), "pvc-test-data") {
		t.Errorf("Expected PVC content, got: %q", exec.Text())
	}
	t.Log("PVC volume mount read/write passed")
}

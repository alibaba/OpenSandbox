//go:build e2e

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func TestFilesystem_GetFileInfo(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer sb.Kill(context.Background())

	info, err := sb.GetFileInfo(ctx, "/etc/os-release")
	if err != nil {
		t.Fatalf("GetFileInfo: %v", err)
	}

	fi, ok := info["/etc/os-release"]
	if !ok {
		t.Fatal("Expected /etc/os-release in result")
	}
	if fi.Size == 0 {
		t.Error("Expected non-zero file size")
	}
	t.Logf("File info: path=%s size=%d owner=%s", fi.Path, fi.Size, fi.Owner)
}

func TestFilesystem_WriteAndReadViaCommand(t *testing.T) {
	config := getConnectionConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sb, err := opensandbox.CreateSandbox(ctx, config, opensandbox.SandboxCreateOptions{
		Image: getSandboxImage(),
	})
	if err != nil {
		t.Fatalf("CreateSandbox: %v", err)
	}
	defer sb.Kill(context.Background())

	// Write via command
	exec, err := sb.RunCommand(ctx, `echo "hello from go e2e" > /tmp/test-go-e2e.txt`, nil)
	if err != nil {
		t.Fatalf("Write file: %v", err)
	}
	if exec.ExitCode != nil && *exec.ExitCode != 0 {
		t.Fatalf("Write file exit code: %d", *exec.ExitCode)
	}

	// Read back via command
	exec, err = sb.RunCommand(ctx, "cat /tmp/test-go-e2e.txt", nil)
	if err != nil {
		t.Fatalf("Read file: %v", err)
	}

	text := exec.Text()
	if text == "" {
		t.Error("Expected non-empty file content")
	}
	t.Logf("File content: %s", text)

	// Verify with GetFileInfo
	info, err := sb.GetFileInfo(ctx, "/tmp/test-go-e2e.txt")
	if err != nil {
		t.Fatalf("GetFileInfo: %v", err)
	}
	fi, ok := info["/tmp/test-go-e2e.txt"]
	if !ok {
		t.Fatal("Expected file in result")
	}
	if fi.Size == 0 {
		t.Error("Expected non-zero file size")
	}
	t.Logf("Written file size: %d", fi.Size)
}

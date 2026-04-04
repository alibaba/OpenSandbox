//go:build e2e

package tests

import (
	"io"
	"strings"
	"testing"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
)

func TestFilesystem_GetFileInfo(t *testing.T) {
	ctx, sb := createTestSandbox(t)

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

func TestFilesystem_WriteReadDelete(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	// Write via command
	exec, err := sb.RunCommand(ctx, `echo "go-e2e-content" > /tmp/test-rw.txt`, nil)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if exec.ExitCode != nil && *exec.ExitCode != 0 {
		t.Fatalf("Write exit code: %d", *exec.ExitCode)
	}

	// Read back via command
	exec, err = sb.RunCommand(ctx, "cat /tmp/test-rw.txt", nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(exec.Text(), "go-e2e-content") {
		t.Errorf("Expected content, got: %q", exec.Text())
	}

	// GetFileInfo
	info, err := sb.GetFileInfo(ctx, "/tmp/test-rw.txt")
	if err != nil {
		t.Fatalf("GetFileInfo: %v", err)
	}
	if _, ok := info["/tmp/test-rw.txt"]; !ok {
		t.Fatal("File not found via GetFileInfo")
	}

	// Delete
	err = sb.DeleteFiles(ctx, []string{"/tmp/test-rw.txt"})
	if err != nil {
		t.Fatalf("DeleteFiles: %v", err)
	}

	// Verify deleted
	exec, err = sb.RunCommand(ctx, "test -f /tmp/test-rw.txt && echo exists || echo gone", nil)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !strings.Contains(exec.Text(), "gone") {
		t.Errorf("File should be deleted, got: %q", exec.Text())
	}
	t.Log("Write/Read/Delete cycle passed")
}

func TestFilesystem_MoveFiles(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	// Create source file
	sb.RunCommand(ctx, `echo "move-me" > /tmp/move-src.txt`, nil)

	// Move
	err := sb.MoveFiles(ctx, opensandbox.MoveRequest{
		{Src: "/tmp/move-src.txt", Dest: "/tmp/move-dst.txt"},
	})
	if err != nil {
		t.Fatalf("MoveFiles: %v", err)
	}

	// Verify destination exists
	exec, err := sb.RunCommand(ctx, "cat /tmp/move-dst.txt", nil)
	if err != nil {
		t.Fatalf("Read moved file: %v", err)
	}
	if !strings.Contains(exec.Text(), "move-me") {
		t.Errorf("Moved file content mismatch: %q", exec.Text())
	}
	t.Log("MoveFiles passed")
}

func TestFilesystem_Directories(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	// Create directory
	err := sb.CreateDirectory(ctx, "/tmp/test-dir-e2e", 755)
	if err != nil {
		t.Fatalf("CreateDirectory: %v", err)
	}

	// Verify exists
	exec, err := sb.RunCommand(ctx, "test -d /tmp/test-dir-e2e && echo yes || echo no", nil)
	if err != nil {
		t.Fatalf("Verify dir: %v", err)
	}
	if !strings.Contains(exec.Text(), "yes") {
		t.Error("Directory should exist")
	}

	// Delete directory
	err = sb.DeleteDirectory(ctx, "/tmp/test-dir-e2e")
	if err != nil {
		t.Fatalf("DeleteDirectory: %v", err)
	}
	t.Log("Directory create/delete passed")
}

func TestFilesystem_SearchFiles(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	results, err := sb.SearchFiles(ctx, "/etc", "*.conf")
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	t.Logf("Found %d files matching *.conf in /etc", len(results))
}

func TestFilesystem_DownloadFile(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	rc, err := sb.DownloadFile(ctx, "/etc/os-release", "")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Read download: %v", err)
	}
	if len(data) == 0 {
		t.Error("Downloaded file is empty")
	}
	t.Logf("Downloaded %d bytes", len(data))
}

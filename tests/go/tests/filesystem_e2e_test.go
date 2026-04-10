package tests

import (
	"io"
	"testing"

	"github.com/alibaba/OpenSandbox/sdks/sandbox/go/opensandbox"
	"github.com/stretchr/testify/require"
)

func TestFilesystem_GetFileInfo(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	info, err := sb.GetFileInfo(ctx, "/etc/os-release")
	require.NoError(t, err)

	fi, ok := info["/etc/os-release"]
	require.True(t, ok, "expected /etc/os-release in result")
	require.NotZero(t, fi.Size, "expected non-zero file size")
	t.Logf("File info: path=%s size=%d owner=%s", fi.Path, fi.Size, fi.Owner)
}

func TestFilesystem_WriteReadDelete(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	// Write via command
	exec, err := sb.RunCommand(ctx, `echo "go-e2e-content" > /tmp/test-rw.txt`, nil)
	require.NoError(t, err)
	if exec.ExitCode != nil {
		require.Equal(t, 0, *exec.ExitCode, "write exit code")
	}

	// Read back via command
	exec, err = sb.RunCommand(ctx, "cat /tmp/test-rw.txt", nil)
	require.NoError(t, err)
	require.Contains(t, exec.Text(), "go-e2e-content")

	// GetFileInfo
	info, err := sb.GetFileInfo(ctx, "/tmp/test-rw.txt")
	require.NoError(t, err)
	_, ok := info["/tmp/test-rw.txt"]
	require.True(t, ok, "file not found via GetFileInfo")

	// Delete
	err = sb.DeleteFiles(ctx, []string{"/tmp/test-rw.txt"})
	require.NoError(t, err)

	// Verify deleted
	exec, err = sb.RunCommand(ctx, "test -f /tmp/test-rw.txt && echo exists || echo gone", nil)
	require.NoError(t, err)
	require.Contains(t, exec.Text(), "gone")
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
	require.NoError(t, err)

	// Verify destination exists
	exec, err := sb.RunCommand(ctx, "cat /tmp/move-dst.txt", nil)
	require.NoError(t, err)
	require.Contains(t, exec.Text(), "move-me")
	t.Log("MoveFiles passed")
}

func TestFilesystem_Directories(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	// Create directory
	err := sb.CreateDirectory(ctx, "/tmp/test-dir-e2e", 755)
	require.NoError(t, err)

	// Verify exists
	exec, err := sb.RunCommand(ctx, "test -d /tmp/test-dir-e2e && echo yes || echo no", nil)
	require.NoError(t, err)
	require.Contains(t, exec.Text(), "yes")

	// Delete directory
	err = sb.DeleteDirectory(ctx, "/tmp/test-dir-e2e")
	require.NoError(t, err)
	t.Log("Directory create/delete passed")
}

func TestFilesystem_SearchFiles(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	results, err := sb.SearchFiles(ctx, "/etc", "*.conf")
	require.NoError(t, err)
	t.Logf("Found %d files matching *.conf in /etc", len(results))
}

func TestFilesystem_DownloadFile(t *testing.T) {
	ctx, sb := createTestSandbox(t)

	rc, err := sb.DownloadFile(ctx, "/etc/os-release", "")
	require.NoError(t, err)
	defer rc.Close()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NotEmpty(t, data, "downloaded file is empty")
	t.Logf("Downloaded %d bytes", len(data))
}

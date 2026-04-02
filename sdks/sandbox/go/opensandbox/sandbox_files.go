package opensandbox

import (
	"context"
	"fmt"
	"io"
)

// GetFileInfo retrieves file metadata.
func (s *Sandbox) GetFileInfo(ctx context.Context, path string) (map[string]FileInfo, error) {
	if s.execd == nil {
		return nil, fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.GetFileInfo(ctx, path)
}

// DeleteFiles deletes one or more files from the sandbox.
func (s *Sandbox) DeleteFiles(ctx context.Context, paths []string) error {
	if s.execd == nil {
		return fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.DeleteFiles(ctx, paths)
}

// MoveFiles renames or moves files.
func (s *Sandbox) MoveFiles(ctx context.Context, req MoveRequest) error {
	if s.execd == nil {
		return fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.MoveFiles(ctx, req)
}

// SearchFiles searches for files matching a pattern.
func (s *Sandbox) SearchFiles(ctx context.Context, dir, pattern string) ([]FileInfo, error) {
	if s.execd == nil {
		return nil, fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.SearchFiles(ctx, dir, pattern)
}

// SetPermissions changes file permissions.
func (s *Sandbox) SetPermissions(ctx context.Context, req PermissionsRequest) error {
	if s.execd == nil {
		return fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.SetPermissions(ctx, req)
}

// UploadFile uploads a local file to the sandbox.
func (s *Sandbox) UploadFile(ctx context.Context, localPath, remotePath string) error {
	if s.execd == nil {
		return fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.UploadFile(ctx, localPath, remotePath)
}

// DownloadFile downloads a file from the sandbox.
func (s *Sandbox) DownloadFile(ctx context.Context, remotePath, rangeHeader string) (io.ReadCloser, error) {
	if s.execd == nil {
		return nil, fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.DownloadFile(ctx, remotePath, rangeHeader)
}

// CreateDirectory creates a directory in the sandbox.
// Mode is octal digits as int (e.g. 755 for rwxr-xr-x).
func (s *Sandbox) CreateDirectory(ctx context.Context, path string, mode int) error {
	if s.execd == nil {
		return fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.CreateDirectory(ctx, path, mode)
}

// DeleteDirectory deletes a directory and its contents.
func (s *Sandbox) DeleteDirectory(ctx context.Context, path string) error {
	if s.execd == nil {
		return fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.DeleteDirectory(ctx, path)
}

// ReplaceInFiles performs text replacement in files.
func (s *Sandbox) ReplaceInFiles(ctx context.Context, req ReplaceRequest) error {
	if s.execd == nil {
		return fmt.Errorf("opensandbox: execd client not initialized")
	}
	return s.execd.ReplaceInFiles(ctx, req)
}

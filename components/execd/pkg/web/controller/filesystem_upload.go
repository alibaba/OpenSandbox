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

package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/log"
	"github.com/alibaba/opensandbox/execd/pkg/util/pathutil"
	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

// UploadFile uploads files with metadata to specified paths
func (c *FilesystemController) UploadFile() {
	rec := beginFilesystemMetric("upload")
	defer rec.Finish(c.basicController)

	form, err := c.ctx.MultipartForm()
	if err != nil || form == nil {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeInvalidFile,
			"multipart form is empty",
		)
		return
	}

	metadataParts := form.File["metadata"]
	fileParts := form.File["file"]

	if len(metadataParts) == 0 {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeInvalidFileMetadata,
			"metadata file is missing",
		)
		return
	}

	if len(fileParts) == 0 {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeInvalidFileContent,
			"file is missing",
		)
		return
	}

	if len(metadataParts) != len(fileParts) {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeInvalidFile,
			fmt.Sprintf("metadata and file count mismatch: %d vs %d", len(metadataParts), len(fileParts)),
		)
		return
	}

	for i := range metadataParts {
		metadataHeader := metadataParts[i]
		metadataFile, err := metadataHeader.Open()
		if err != nil {
			c.RespondError(
				http.StatusBadRequest,
				model.ErrorCodeInvalidFileMetadata,
				fmt.Sprintf("error opening metadata file. %v", err),
			)
			return
		}

		metaBytes, err := io.ReadAll(metadataFile)
		metadataFile.Close()
		if err != nil {
			c.RespondError(
				http.StatusBadRequest,
				model.ErrorCodeInvalidFileMetadata,
				fmt.Sprintf("error reading metadata content. %v", err),
			)
			return
		}

		var meta model.FileMetadata
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			c.RespondError(
				http.StatusBadRequest,
				model.ErrorCodeInvalidFileMetadata,
				fmt.Sprintf("invalid metadata format. %v", err),
			)
			return
		}

		targetPath := meta.Path
		if targetPath == "" {
			c.RespondError(
				http.StatusBadRequest,
				model.ErrorCodeInvalidFileMetadata,
				"metadata path is empty",
			)
			return
		}
		resolvedPath, err := pathutil.ExpandPath(targetPath)
		if err != nil {
			c.RespondError(
				http.StatusInternalServerError,
				model.ErrorCodeRuntimeError,
				fmt.Sprintf("error resolving target path %s. %v", targetPath, err),
			)
			return
		}

		targetDir := filepath.Dir(resolvedPath)
		if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
			c.RespondError(
				http.StatusInternalServerError,
				model.ErrorCodeRuntimeError,
				fmt.Sprintf("error creating target directory %s. %v", targetDir, err),
			)
			return
		}

		fileHeader := fileParts[i]
		file, err := fileHeader.Open()
		if err != nil {
			c.RespondError(
				http.StatusInternalServerError,
				model.ErrorCodeRuntimeError,
				fmt.Sprintf("error opening file %s. %v", fileHeader.Filename, err),
			)
			return
		}

		dst, err := os.OpenFile(resolvedPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
		if err != nil {
			file.Close()
			c.RespondError(
				http.StatusInternalServerError,
				model.ErrorCodeRuntimeError,
				fmt.Sprintf("error opening destination file %s. %v", resolvedPath, err),
			)
			return
		}

		if _, err := io.Copy(dst, file); err != nil {
			dst.Close()
			file.Close()
			c.RespondError(
				http.StatusInternalServerError,
				model.ErrorCodeRuntimeError,
				fmt.Sprintf("error copying file %s. %v", resolvedPath, err),
			)
			return
		}

		if err := dst.Sync(); err != nil {
			log.Error("failed to sync target file: %v", err)
		}
		if err := dst.Close(); err != nil {
			log.Error("failed to close target file: %v", err)
		}
		file.Close()

		// fsync parent directory so the new dirent is durable and visible on
		// weakly-coherent filesystems (virtio-fs, 9pfs, etc.). Best-effort:
		// some filesystems return ENOTSUP for directory fsync.
		if d, err := os.Open(targetDir); err == nil {
			if err := d.Sync(); err != nil {
				log.Warning("failed to sync parent dir %s: %v", targetDir, err)
			}
			_ = d.Close()
		}

		// Apply permissions with one retry to absorb metadata-propagation
		// delay on weakly-coherent filesystems (virtio-fs, 9pfs). ChmodFile
		// always invokes chown under the hood, so a freshly-created dirent
		// that has not yet propagated will surface as ENOENT here even
		// though the file is fully written and synced.
		chmodErr := ChmodFile(resolvedPath, meta.Permission)
		if chmodErr != nil {
			time.Sleep(20 * time.Millisecond)
			chmodErr = ChmodFile(resolvedPath, meta.Permission)
		}
		if chmodErr != nil {
			c.RespondError(
				http.StatusInternalServerError,
				model.ErrorCodeRuntimeError,
				fmt.Sprintf("error chmoding file %s. %v", resolvedPath, chmodErr),
			)
			return
		}
	}

	rec.MarkSuccess()
	c.RespondSuccess(nil)
}

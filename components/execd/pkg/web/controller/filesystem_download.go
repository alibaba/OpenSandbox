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
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/alibaba/opensandbox/execd/pkg/web/model"
)

// DownloadFile serves a file for download with support for range requests
func (c *FilesystemController) DownloadFile() {
	filePath := c.GetString("path")
	if filePath == "" {
		c.RespondError(
			http.StatusBadRequest,
			model.ErrorCodeMissingQuery,
			"missing query parameter 'path'",
		)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		c.handleFileError(err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		c.RespondError(
			http.StatusInternalServerError,
			model.ErrorCodeRuntimeError,
			fmt.Sprintf("error getting file stat info: %s. %v", filePath, err),
		)
		return
	}

	c.Ctx.Output.Header("Content-Type", "application/octet-stream")
	c.Ctx.Output.Header("Content-Disposition", "attachment; filename="+filepath.Base(filePath))
	c.Ctx.Output.Header("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))

	if rangeHeader := c.Ctx.Input.Header("Range"); rangeHeader != "" {
		ranges, err := ParseRange(rangeHeader, fileInfo.Size())
		if err != nil {
			c.RespondError(
				http.StatusRequestedRangeNotSatisfiable,
				model.ErrorCodeUnknown,
			)
			return
		}
		if len(ranges) > 0 {
			r := ranges[0]
			c.Ctx.Output.SetStatus(http.StatusPartialContent)
			c.Ctx.Output.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", r.start, r.start+r.length-1, fileInfo.Size()))
			c.Ctx.Output.Header("Content-Length", strconv.FormatInt(r.length, 10))

			_, _ = file.Seek(r.start, io.SeekStart)
			_, _ = io.CopyN(c.Ctx.ResponseWriter, file, r.length)
			return
		}
	}

	http.ServeContent(c.Ctx.ResponseWriter, c.Ctx.Request, filepath.Base(filePath), fileInfo.ModTime(), file)
}

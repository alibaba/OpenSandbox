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

package e2e_runtime

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// getProjectRoot returns the project root directory by trimming the test subdirectory
func getProjectRoot() string {
	wd, _ := os.Getwd()
	// Remove test/e2e_runtime/* or test/e2e/* from path to get project root
	projectRoot := strings.ReplaceAll(wd, "/test/e2e_runtime/", "/test/")
	projectRoot = strings.ReplaceAll(projectRoot, "/test/e2e/", "/test/")
	return strings.TrimSuffix(projectRoot, "/test/e2e")
}

// RenderTemplate renders a YAML template file with given data
// templateFile is relative to e2e_runtime/ directory (e.g. "gvisor/testdata/pool.yaml")
func RenderTemplate(templateFile string, data map[string]interface{}) (string, error) {
	// Use relative path from project root (where tests are run)
	fullPath := filepath.Join("test", "e2e_runtime", templateFile)
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

// By outputs a message to GinkgoWriter (placeholder for use in test suites)
func By(message string) {
	fmt.Printf("STEP: %s\n", message)
}

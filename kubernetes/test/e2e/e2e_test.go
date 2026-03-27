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
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/alibaba/OpenSandbox/sandbox-k8s/test/utils"
	. "github.com/onsi/ginkgo/v2"
)

// namespace where the project is deployed in
const namespace = "opensandbox-system"

var _ = Describe("Manager", Ordered, func() {})

// renderTemplate renders a YAML template file with the given data.
func renderTemplate(templateFile string, data map[string]interface{}) (string, error) {
	dir, err := utils.GetProjectDir()
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(dir, "test", "e2e", templateFile)
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

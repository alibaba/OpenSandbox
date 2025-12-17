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
	"bytes"
	"net/http"
)

type mockOutput struct {
	buffer     *bytes.Buffer
	statusCode int
	header     http.Header
}

func (m *mockOutput) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockOutput) Write(b []byte) (int, error) {
	return m.buffer.Write(b)
}

func (m *mockOutput) WriteHeader(code int) {
	m.statusCode = code
}

func (m *mockOutput) Status() int {
	return m.statusCode
}

func (m *mockOutput) Body() []byte {
	return m.buffer.Bytes()
}

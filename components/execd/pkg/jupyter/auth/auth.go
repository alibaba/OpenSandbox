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

package auth

import (
	"fmt"
	"net/url"
)

// Auth represents authentication configuration.
type Auth struct {
	Token    string
	Username string
	Password string
}

// NewTokenAuth builds a token-based config.
func NewTokenAuth(token string) *Auth {
	return &Auth{
		Token: token,
	}
}

// NewBasicAuth builds a basic-auth config.
func NewBasicAuth(username, password string) *Auth {
	return &Auth{
		Username: username,
		Password: password,
	}
}

// Validate reports which auth mode is configured.
func (a *Auth) Validate() string {
	if a.Token != "" {
		return "token"
	}
	if a.Username != "" {
		return "basic"
	}
	return "none"
}

// AddAuthToURL appends token query parameters to the URL.
func (a *Auth) AddAuthToURL(baseURL string) (string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	query := parsedURL.Query()

	if a.Token != "" {
		query.Set("token", a.Token)
	}

	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

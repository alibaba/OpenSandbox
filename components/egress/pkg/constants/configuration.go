// Copyright 2026 Alibaba Group Holding Ltd.
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

package constants

const (
	EnvBlockDoH443    = "OPENSANDBOX_EGRESS_BLOCK_DOH_443"
	EnvDoHBlocklist   = "OPENSANDBOX_EGRESS_DOH_BLOCKLIST" // comma-separated IP/CIDR
	EnvEgressMode     = "OPENSANDBOX_EGRESS_MODE"          // dns | dns+nft
	EnvEgressHTTPAddr = "OPENSANDBOX_EGRESS_HTTP_ADDR"
	EnvEgressToken    = "OPENSANDBOX_EGRESS_TOKEN"
	EnvEgressRules    = "OPENSANDBOX_EGRESS_RULES"
)

const (
	PolicyDnsOnly = "dns"
	PolicyDnsNft  = "dns+nft"
)

const (
	DefaultEgressServerAddr = ":18080"
)

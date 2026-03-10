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

package dnsproxy

import (
	"net/netip"
	"reflect"
	"sync"
	"testing"

	"github.com/alibaba/opensandbox/egress/pkg/constants"
)

func resetNameserverExemptCache(t *testing.T) {
	t.Helper()
	exemptAddrs = nil
	exemptSet = nil
	exemptListOnce = sync.Once{}
}

func TestParseNameserverExemptList_IPOnly(t *testing.T) {
	t.Setenv(constants.EnvNameserverExempt, "1.1.1.1, 2001:db8::1 ,invalid, 10.0.0.0/8, ,")
	resetNameserverExemptCache(t)

	got := ParseNameserverExemptList()
	want := []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2001:db8::1")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseNameserverExemptList() = %v, want %v", got, want)
	}

	// Cached result should stay the same on subsequent calls.
	if got2 := ParseNameserverExemptList(); !reflect.DeepEqual(got2, want) {
		t.Fatalf("cached ParseNameserverExemptList() = %v, want %v", got2, want)
	}
}

func TestUpstreamInExemptList_IPOnly(t *testing.T) {
	t.Setenv(constants.EnvNameserverExempt, "1.1.1.1,2001:db8::1")
	resetNameserverExemptCache(t)

	if !UpstreamInExemptList("1.1.1.1") {
		t.Fatalf("expected IPv4 upstream to be exempt")
	}
	if !UpstreamInExemptList("2001:db8::1") {
		t.Fatalf("expected IPv6 upstream to be exempt")
	}
	if UpstreamInExemptList("10.0.0.2") {
		t.Fatalf("unexpected exempt match for non-listed IP")
	}
	if UpstreamInExemptList("not-an-ip") {
		t.Fatalf("invalid IP string should not match")
	}
}

func TestUpstreamInExemptList_CIDRIgnored(t *testing.T) {
	t.Setenv(constants.EnvNameserverExempt, "10.0.0.0/24")
	resetNameserverExemptCache(t)

	if got := ParseNameserverExemptList(); len(got) != 0 {
		t.Fatalf("CIDR should be ignored in exempt list, got %v", got)
	}
	if UpstreamInExemptList("10.0.0.5") {
		t.Fatalf("CIDR should not make upstream exempt")
	}
}

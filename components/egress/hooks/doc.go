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

// Package hooks is a blank-import anchor for egress startup extensions.
//
// Add Go files that call startup.Register or startup.RegisterFunc from init().
// Hooks run in RunPost — after transparent mitmdump setup returns (see main).
//
//	func init() {
//		startup.RegisterFunc("example", func(ctx context.Context) error { return nil })
//	}
//
// main imports this package so those init functions run before main().
package hooks

// Copyright Project Contour Authors
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

package envoy

import "syscall"

// We only support Envoy on Linux so always configure Linux TCP keep-alive
// socket options regardless of the platform that Contour is running on.
// nolint:revive
const (
	TCP_KEEPIDLE  = 0x4 // Linux syscall.TCP_KEEPIDLE
	TCP_KEEPINTVL = 0x5 // Linux syscall.TCP_KEEPINTVL
	TCP_KEEPCNT   = 0x6 // Linux syscall.TCP_KEEPCNT

	// The following are Linux syscall constants for all
	// architectures except MIPS.
	SOL_SOCKET   = 0x1
	SO_KEEPALIVE = 0x9

	// IPPROTO_TCP has the same value across Go platforms, but
	// is defined here for consistency.
	IPPROTO_TCP = syscall.IPPROTO_TCP
)

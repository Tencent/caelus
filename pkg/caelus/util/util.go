/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 *
 * You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package util

/*
#include <unistd.h>
#include <sys/types.h>
#include <pwd.h>
#include <stdlib.h>
*/
import "C"
import (
	"net"
	"sync/atomic"
	"unsafe"
)

var (
	// nodeName store the current node name
	nodeName unsafe.Pointer

	// nodeIP store the current node ip
	nodeIP unsafe.Pointer

	// InHostNamespace show if the current namespace is host
	InHostNamespace bool

	cpuTicksPerSecond int64

	// SilenceMode indicate do not running offline jobs
	SilenceMode bool
)

// NodeName return current node name
func NodeName() string {
	c := (*string)(atomic.LoadPointer(&nodeName))
	if c != nil {
		return *c
	}
	return ""
}

// SetNodeName set current node name
func SetNodeName(name string) {
	atomic.StorePointer(
		&nodeName, unsafe.Pointer(&name))
}

// NodeIP return current node ip
func NodeIP() string {
	c := (*string)(atomic.LoadPointer(&nodeIP))
	if c != nil {
		return *c
	}
	return ""
}

// SetNodeIP set current node ip
func SetNodeIP(ip string) {
	atomic.StorePointer(
		&nodeIP, unsafe.Pointer(&ip))
}

// MatchIP check if the input string is an ip address
func MatchIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// GetClockTicksPerSecond return clock ticks per second from unix
func GetClockTicksPerSecond() int64 {
	if t := atomic.LoadInt64(&cpuTicksPerSecond); t > 0 {
		return t
	}
	atomic.StoreInt64(&cpuTicksPerSecond, int64(C.sysconf(C._SC_CLK_TCK)))
	return cpuTicksPerSecond
}

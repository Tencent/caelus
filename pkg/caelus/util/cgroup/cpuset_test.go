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

package cgroup

import (
	"os"
	"testing"
)

var (
	tmpCpuSetCgroupPath = "/sys/fs/cgroup/cpuset/test"
	tmpCpuSetSub        = "test"
)

// TestGetCpuSet test getting cpuset value
func TestGetCpuSet(t *testing.T) {
	err := os.MkdirAll(tmpCpuSetCgroupPath, 0711)
	if err != nil {
		t.Skipf("create cpuset path(%s) failed: %v", tmpCpuSetCgroupPath, err)
	}
	defer os.RemoveAll(tmpCpuSetCgroupPath)

	cpusetStr := "0,2-3"
	cpusetSlice := []int{0, 2, 3}
	cpusetStrSlice := []string{"0", "2", "3"}
	err = WriteCpuSetCores(tmpCpuSetSub, cpusetSlice)
	if err != nil {
		t.Fatalf("write cpuset value(%s) to %s err: %v", cpusetStr, tmpCpuSetCgroupPath, err)
	}

	resultSlice, err := GetCpuSet(tmpCpuSetSub, true)
	if err != nil {
		t.Fatalf("get cpuset value from %s failed: %v", tmpCpuSetCgroupPath, err)
	}
	for i, v := range cpusetStrSlice {
		if v != resultSlice[i] {
			t.Fatalf("get unexpect cpuset value from %s, expect %v, got %v",
				tmpCpuSetCgroupPath, cpusetStrSlice, resultSlice)
		}
	}

	resultStr, err := GetCpuSet(tmpCpuSetSub, false)
	if err != nil {
		t.Fatalf("get cpuset value from %s failed: %v", tmpCpuSetCgroupPath, err)
	}
	if resultStr[0] != cpusetStr {
		t.Fatalf("get unexpect cpuset value from %s, expect %s, got %s",
			tmpCpuSetCgroupPath, cpusetStr, resultStr)
	}
}

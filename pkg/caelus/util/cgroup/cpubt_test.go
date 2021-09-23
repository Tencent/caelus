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
	"fmt"
	"github.com/shirou/gopsutil/cpu"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"testing"
)

var (
	tmpBtCgroupPath      = "/sys/fs/cgroup/cpu,cpuacct/test"
	tmpBtSubPath         = "test"
	tmpBtCgroupPathChild = "/sys/fs/cgroup/cpu,cpuacct/test/test1"
)

// TestEnableCPUChildOffline test enable cpu offline feature for children path
func TestEnableCPUChildOffline(t *testing.T) {
	if !CPUOfflineSupported() {
		t.Skipf("bt not supported")
	}

	err := os.MkdirAll(tmpBtCgroupPathChild, 0711)
	if err != nil {
		t.Skipf("create cpu cgroup path %s err: %v", tmpBtCgroupPathChild, err)
	}
	defer func() {
		os.RemoveAll(tmpBtCgroupPathChild)
		os.RemoveAll(tmpBtCgroupPath)
	}()

	err = CPUOfflineSet(tmpBtSubPath, true)
	if err != nil {
		t.Fatalf("set offline feature for path %s err: %v", tmpBtSubPath, err)
	}

	err = CPUChildOfflineSet(tmpBtSubPath, true)
	if err != nil {
		t.Fatalf("set children offline feature for path %s err: %v", tmpBtSubPath, err)
	}

	valueBytes, err := ioutil.ReadFile(path.Join(tmpBtCgroupPathChild, "cpu.offline"))
	if err != nil {
		t.Fatalf("set children offline feature for path %s err: %v", tmpBtSubPath, err)
	}
	if strings.Trim(string(valueBytes), "\n") != "1" {
		t.Fatalf("children offline feature not enabled")
	}
}

type btTestData struct {
	describe      string
	limitCores    int
	minPercent    int
	expectPercent string
	expectErr     error
}

// TestCPUOfflineLimit test offline limit setting
func TestCPUOfflineLimit(t *testing.T) {
	if !CPUOfflineSupported() {
		t.Skipf("bt not supported")
	}

	// recover origin value
	var originValues = make(map[string]string)
	cpus, err := ioutil.ReadDir(offlinePath)
	if err != nil {
		t.Skipf("read offline path(%s) failed: %v", offlinePath, err)
	}
	for _, f := range cpus {
		p := path.Join(offlinePath, f.Name())
		value, err := ioutil.ReadFile(p)
		if err != nil {
			t.Skipf("read offline file(%s) err: %v", p, err)
			return
		}
		originValues[p] = string(value)
	}
	defer func() {
		for p, v := range originValues {
			ioutil.WriteFile(p, []byte(v), 0664)
		}
	}()

	// get total cpu numbers
	cpuInfo, err := cpu.Info()
	if err != nil {
		t.Skipf("get machine cpu info err: %v", err)
	}
	total := len(cpuInfo)
	half := total / 2
	halfPercent := half * 100 / total

	// all cases
	btCases := []btTestData{
		{
			describe:      "normal test",
			limitCores:    half,
			minPercent:    30,
			expectPercent: fmt.Sprintf("%d", halfPercent),
			expectErr:     nil,
		},
		{
			describe:      "zero test",
			limitCores:    0,
			minPercent:    30,
			expectPercent: "30",
			expectErr:     nil,
		},
		{
			describe:      "min percent test",
			limitCores:    half,
			minPercent:    70,
			expectPercent: "70",
			expectErr:     nil,
		},
		{
			describe:      "exceed test",
			limitCores:    total + 1,
			minPercent:    40,
			expectPercent: "100",
			expectErr:     fmt.Errorf("cpu limit number too big"),
		},
	}

	for _, c := range btCases {
		err := CPUOfflineLimit(c.limitCores, c.minPercent)
		if err != nil {
			if err.Error() != c.expectErr.Error() {
				t.Fatalf("bt test case(%s) failed, expect err is %v, got %v", c.describe, c.expectErr, err)
			}
			continue
		}
		for p := range originValues {
			vByts, err := ioutil.ReadFile(p)
			if err != nil {
				t.Fatalf("read cpu offline file(%s) err: %v", p, err)
			}
			if strings.Trim(string(vByts), "\n") != c.expectPercent {
				t.Fatalf("bt test case(%s) unexpect result, expect %s, got %s",
					c.describe, c.expectPercent, string(vByts))
			}
		}
	}
}

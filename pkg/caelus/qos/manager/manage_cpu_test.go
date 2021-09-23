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

package manager

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"

	"github.com/shirou/gopsutil/cpu"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/sets"
)

type cpuBtTestData struct {
	describe        string
	cpuStatic       bool
	cpusetRecovered bool
	limitCores      int64
	expectPercent   string
}

// TestQosCpuBT_Manage tests cpu bt qos manager
func TestQosCpuBT_Manage(t *testing.T) {
	if !cgroup.CPUOfflineSupported() {
		t.Skipf("cpu qos bt test skipped for not supported")
	}
	total, err := getTotalCpus()
	if err != nil {
		t.Skipf("cpu qos bt test skipped for get total cpus err: %v", err)
	}

	// recover origin value
	procOffline := "/proc/offline/"
	var originValues = make(map[string]string)
	cpus, err := ioutil.ReadDir(procOffline)
	if err != nil {
		t.Skipf("read offline path(%s) failed: %v", procOffline, err)
	}
	for _, f := range cpus {
		p := path.Join(procOffline, f.Name())
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

	minCpuBtPercent = 50
	testCases := []cpuBtTestData{
		{
			describe:        "normal test",
			cpuStatic:       false,
			cpusetRecovered: true,
			limitCores:      total,
			expectPercent:   "100",
		},
		{
			describe:        "min limit test",
			cpuStatic:       false,
			cpusetRecovered: true,
			limitCores:      1,
			expectPercent:   fmt.Sprintf("%d", minCpuBtPercent),
		},
		{
			describe:        "static test",
			cpuStatic:       true,
			cpusetRecovered: true,
			limitCores:      total,
			expectPercent:   "100",
		},
		{
			describe:        "cpuset recovered",
			cpuStatic:       false,
			cpusetRecovered: false,
			limitCores:      total,
			expectPercent:   "100",
		},
	}

	offlineCg := "/sys/fs/cgroup/cpu,cpuacct" + types.CgroupOffline
	// offlineCpusetCg should be under the offlineCg path, we not creates it as this just for test
	offlineCpusetCg := "/sys/fs/cgroup/cpuset/offlinetest"
	offlineCpusetCgInRoot := "/offlinetest"

	for _, tc := range testCases {
		btQos := &qosCpuBT{
			kubeletStatic: tc.cpuStatic,
		}
		cpusetRecovered = tc.cpusetRecovered

		func() {
			existed, err := mkdirCgPath(offlineCg)
			if err != nil {
				t.Fatalf("mkdir offline cgroup %s err: %v", offlineCg, err)
			}
			if !existed {
				defer os.RemoveAll(offlineCg)
			}

			existed, err = mkdirCgPath(offlineCpusetCg)
			if err != nil {
				t.Fatalf("mkdir offline cgroup %s err: %v", offlineCpusetCg, err)
			}
			if !existed {
				defer os.RemoveAll(offlineCpusetCg)
			}

			btQos.Manage(&CgroupResourceConfig{
				Resources: v1.ResourceList{
					v1.ResourceCPU: *resource.NewMilliQuantity(tc.limitCores*1000, resource.DecimalSI),
				},
				OfflineCgroups: []string{
					offlineCpusetCgInRoot,
				},
			})

			valueBytes, err := ioutil.ReadFile(path.Join(offlineCg, "cpu.offline"))
			if err != nil {
				t.Fatalf("read offline cgroup %s err: %v", offlineCg, err)
			}
			if strings.Trim(string(valueBytes), "\n") != "1" {
				t.Fatalf("cpu qos bt test case(%s) failed, offline(%s) not enabled", tc.describe, offlineCg)
			}

			for p := range originValues {
				vByts, err := ioutil.ReadFile(p)
				if err != nil {
					t.Fatalf("read cpu offline file(%s) err: %v", p, err)
				}
				if strings.Trim(string(vByts), "\n") != tc.expectPercent {
					t.Fatalf("cpu qos bt test case(%s) unexpect result, expect %s, got %s",
						tc.describe, tc.expectPercent, string(vByts))
				}
			}

			cpusetStr, err := readCpuSetCgroup(offlineCpusetCg)
			if err != nil {
				t.Fatalf("read cpuset cgroup %s err: %v", offlineCpusetCg, err)
			}
			if (!tc.cpuStatic && tc.cpusetRecovered) && len(cpusetStr) != 0 {
				t.Fatalf("cpu qos bt test case(%s) failed, static is false, should not set cpusets: %s",
					tc.describe, cpusetStr)
			}
			if (tc.cpuStatic || !tc.cpusetRecovered) && len(cpusetStr) == 0 {
				t.Fatalf("cpu qos bt test case(%s) failed, static is true, should set cpusets, got null",
					tc.describe)
			}
		}()
	}
}

type cpuSetTestData struct {
	describe      string
	reserved      sets.Int
	limit         int64
	onlineIsolate bool
	expect        struct {
		offline string
		online  string
	}
}

// TestQosCpuSet_Manage tests cpuset qos manager
func TestQosCpuSet_Manage(t *testing.T) {
	total, err := getTotalCpus()
	if err != nil {
		t.Skipf("cpu qos cpuset skipped for get total cpu err: %v", err)
	}
	lastCoreStr := fmt.Sprintf("%d", total-1)
	lastSecCoreStr := fmt.Sprintf("%d", total-2)
	leftCoreStr := fmt.Sprintf("0-%d", total-2)
	if total == 2 {
		leftCoreStr = "0"
	}

	testCases := []cpuSetTestData{
		{
			describe:      "no reserved",
			reserved:      sets.NewInt(),
			limit:         1,
			onlineIsolate: false,
			expect: struct {
				offline string
				online  string
			}{offline: lastCoreStr, online: ""},
		},
		{
			describe:      "has reserved",
			reserved:      sets.NewInt([]int{int(total) - 1}...),
			limit:         1,
			onlineIsolate: false,
			expect: struct {
				offline string
				online  string
			}{offline: lastSecCoreStr, online: ""},
		},
		{
			describe:      "online isolate enable",
			reserved:      sets.NewInt(),
			limit:         1,
			onlineIsolate: true,
			expect: struct {
				offline string
				online  string
			}{offline: lastCoreStr, online: leftCoreStr},
		},
	}

	cpusetCg := "/sys/fs/cgroup/cpuset"
	offlineCgInRoot := "/offlinetest"
	onlineCgInRoot := "/onlinetest"
	offlineCg := path.Join(cpusetCg, offlineCgInRoot)
	onlineCg := path.Join(cpusetCg, onlineCgInRoot)
	for _, tc := range testCases {
		qosCpuset := &qosCpuSet{
			onlineIsolate:  tc.onlineIsolate,
			reserved:       tc.reserved,
			lastOfflineCgs: newCgroupPaths(),
			lastOnlineCgs:  newCgroupPaths(),
		}

		func() {
			existed, err := mkdirCgPath(offlineCg)
			if err != nil {
				t.Fatalf("mkdir offline cgroup %s err: %v", offlineCg, err)
			}
			if !existed {
				defer os.RemoveAll(offlineCg)
			}

			existed, err = mkdirCgPath(onlineCg)
			if err != nil {
				t.Fatalf("mkdir online cgroup %s err: %v", onlineCg, err)
			}
			if !existed {
				defer os.RemoveAll(onlineCg)
			}

			qosCpuset.Manage(&CgroupResourceConfig{
				Resources: v1.ResourceList{
					v1.ResourceCPU: *resource.NewMilliQuantity(tc.limit*1000, resource.DecimalSI),
				},
				OfflineCgroups: []string{
					offlineCgInRoot,
				},
				OnlineCgroups: []string{
					onlineCgInRoot,
				},
			})

			offlineCpusets, err := readCpuSetCgroup(offlineCg)
			if err != nil {
				t.Fatalf("read cpuset cgroup %s err: %v", offlineCg, err)
			}
			if offlineCpusets != tc.expect.offline {
				t.Fatalf("cpu qos cpuset test case(%s) failed, expect offline %s, got %s",
					tc.describe, tc.expect.offline, offlineCpusets)
			}
			onCpusets, err := readCpuSetCgroup(onlineCg)
			if err != nil {
				t.Fatalf("read cpuset cgroup %s err: %v", onlineCg, err)
			}
			if onCpusets != tc.expect.online {
				t.Fatalf("cpu qos cpuset test case(%s) failed, expect online %s, got %s",
					tc.describe, tc.expect.online, onCpusets)
			}
		}()
	}
}

type cpuQuotaTestData struct {
	describe string
	limit    int64
	weight   *uint64
	expect   struct {
		limit  string
		weight string
	}
}

// TestQosCpuQuota_Manage test cpu quota qos manager
func TestQosCpuQuota_Manage(t *testing.T) {
	testCases := []cpuQuotaTestData{
		{
			describe: "quota test",
			limit:    2,
			weight:   nil,
			expect: struct {
				limit  string
				weight string
			}{limit: "2000000", weight: "1024"},
		},
		{
			describe: "weight test",
			limit:    2,
			weight:   uint64Pointer(2),
			expect: struct {
				limit  string
				weight string
			}{limit: "2000000", weight: "2"},
		},
	}

	quotaCgPath := "/sys/fs/cgroup/cpu,cpuacct" + types.CgroupOffline
	offlinePath := "/sys/fs/cgroup/cpu,cpuacct" + types.CgroupOffline + "/test"
	offlinePathInRoot := types.CgroupOffline + "/test"

	for _, tc := range testCases {
		qosQuota := &qosCpuQuota{
			shareWeight:    tc.weight,
			kubeletStatic:  false,
			lastOfflineCgs: newCgroupPaths(),
		}

		func() {
			existed, err := mkdirCgPath(quotaCgPath)
			if err != nil {
				t.Fatalf("mkdir offline cgroup %s err: %v", quotaCgPath, err)
			}
			if !existed {
				defer os.RemoveAll(quotaCgPath)
			}

			existed, err = mkdirCgPath(offlinePath)
			if err != nil {
				t.Fatalf("mkdir online cgroup %s err: %v", offlinePath, err)
			}
			if !existed {
				defer os.RemoveAll(offlinePath)
			}

			qosQuota.Manage(&CgroupResourceConfig{
				Resources: v1.ResourceList{
					v1.ResourceCPU: *resource.NewMilliQuantity(tc.limit*1000, resource.DecimalSI),
				},
				OfflineCgroups: []string{
					offlinePathInRoot,
				},
			})

			quotaBytes, err := ioutil.ReadFile(path.Join(quotaCgPath, "cpu.cfs_quota_us"))
			if err != nil {
				t.Fatalf("read cpu quota for %s err: %v", quotaCgPath, err)
			}
			if strings.Trim(string(quotaBytes), "\n") != tc.expect.limit {
				t.Fatalf("cpu qos quota test case(%s) failed, expect quota: %s, got: %s",
					tc.describe, tc.expect.limit, strings.Trim(string(quotaBytes), "\n"))
			}

			shareBytes, err := ioutil.ReadFile(path.Join(offlinePath, "cpu.shares"))
			if err != nil {
				t.Fatalf("read cpu share for %s err: %v", offlinePath, err)
			}
			if strings.Trim(string(shareBytes), "\n") != tc.expect.weight {
				t.Fatalf("cpu qos quota test case(%s) failed, expect share: %s, got: %s",
					tc.describe, tc.expect.weight, strings.Trim(string(shareBytes), "\n"))
			}
		}()
	}

}

func getTotalCpus() (int64, error) {
	cpuInfo, err := cpu.Info()
	if err != nil {
		return 0, err
	}
	return int64(len(cpuInfo)), nil
}

func readCpuSetCgroup(cgPath string) (value string, err error) {
	data, err := ioutil.ReadFile(filepath.Join(cgPath, "cpuset.cpus"))
	if err != nil {
		return "", err
	}
	return strings.Replace(string(data), "\n", "", -1), nil
}

func mkdirCgPath(cgPath string) (existed bool, err error) {
	existed = true
	_, err = os.Stat(cgPath)
	if err != nil {
		if os.IsNotExist(err) {
			existed = false
			err = os.MkdirAll(cgPath, 0755)
		}
	}

	return existed, err
}

func uint64Pointer(value uint64) *uint64 {
	var p *uint64
	p = new(uint64)
	*p = value

	return p
}

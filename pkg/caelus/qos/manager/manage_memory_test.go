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
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/statestore/cgroup"
	mockstore "github.com/tencent/caelus/pkg/caelus/statestore/mock"
	"github.com/tencent/caelus/pkg/caelus/types"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type memoryQosTestData struct {
	describe string
	limit    int64
	usage    float64
	expect   int
}

// TestQosMemory_Manage tests memory qos manager
func TestQosMemory_Manage(t *testing.T) {
	memoryCgPath := path.Join("/sys/fs/cgroup/memory", types.CgroupOffline)
	_, err := os.Stat(memoryCgPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(memoryCgPath, 0700)
			if err != nil {
				t.Skipf("mkdir cgroup path %s err: %v", memoryCgPath, err)
			}
			defer os.RemoveAll(memoryCgPath)
		} else {
			t.Skipf("check cgroup path %s err: %v", memoryCgPath, err)
		}
	}

	testCases := []memoryQosTestData{
		{
			describe: "limit is ok",
			limit:    12 * 1024 * 1024 * 1024,
			usage:    10 * 1024 * 1024 * 1024,
			expect:   12 * 1024 * 1024 * 1024,
		},
		{
			describe: "limit is not ok, bigger than usage",
			limit:    10.5 * 1024 * 1024 * 1024,
			usage:    10 * 1024 * 1024 * 1024,
			expect:   11806965760,
		},
		{
			describe: "limit is not ok, litter than usage",
			limit:    8 * 1024 * 1024 * 1024,
			usage:    10 * 1024 * 1024 * 1024,
			expect:   11806965760,
		},
	}

	for _, tc := range testCases {
		fakeCgroupStore := &mockstore.MockCgroupStore{
			CgStats: map[string]*cgroupstore.CgroupStats{
				"kubepods": {
					Ref: &cgroupstore.CgroupRef{
						Name: types.CgroupOffline,
					},
					MemoryTotalUsage: tc.usage,
				},
			},
		}
		fakeStStore := mockstore.NewMockStatStore(nil, fakeCgroupStore)
		memQos := &qosMemory{
			stStore: fakeStStore,
		}

		err = memQos.Manage(&CgroupResourceConfig{
			Resources: v1.ResourceList{
				v1.ResourceMemory: *resource.NewQuantity(tc.limit, resource.DecimalSI),
			},
		})
		if err != nil {
			t.Fatalf("memory qos test case(%s) failed, err: %v", tc.describe, err)
		}

		valueBytes, err := ioutil.ReadFile(path.Join(memoryCgPath, "memory.limit_in_bytes"))
		if err != nil {
			t.Fatalf("read memory cgroup path(%s) limit value failed: %v", memoryCgPath, err)
		}
		value := strings.Replace(string(valueBytes), "\n", "", -1)
		limit, err := strconv.Atoi(value)
		if err != nil {
			t.Fatalf("invalid memory limit value(%s): %v", value, err)
		}
		if limit != tc.expect {
			t.Fatalf("memory qos test case(%s) failed, expect %d, got %d", tc.describe, tc.expect, limit)
		}
	}
}

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

package machine

import (
	"fmt"

	"github.com/guillermo/go.procmeminfo"
	"github.com/shirou/gopsutil/cpu"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
)

var (
	// memoryBuffer allow the buffer size when the memory limit value is equal to the usage value
	memoryBuffer = int64(1024 * 1020 * 1024)
	minUsage     = int64(100 * 1024 * 1024)
)

// totalResource is the capacity resource of the node, no lock is ok.
var totalResource = v1.ResourceList{}

// GetTotalResource return node capacity resource
func GetTotalResource() (v1.ResourceList, error) {
	if len(totalResource) == 0 {
		cores, err := cpu.Info()
		if err != nil {
			return totalResource, err
		}
		mem := procmeminfo.MemInfo{}
		mem.Update()
		mems := mem.Total()
		totalResource[v1.ResourceCPU] = *resource.NewMilliQuantity(int64(len(cores)*1000), resource.DecimalSI)
		totalResource[v1.ResourceMemory] = *resource.NewQuantity(int64(mems), resource.DecimalSI)
	}

	return totalResource, nil
}

// GetMemoryCgroupLimitByUsage check limit value and current usage value for memory cgroup,
// and will change limit value if necessary
func GetMemoryCgroupLimitByUsage(limit, usage int64) (newLimit int64, reason string) {
	newLimit = limit
	// if usage is too small(100Mi), just set origin limit value, which min value is 128Mi
	if usage <= minUsage {
		return
	}

	if limit-usage < memoryBuffer {
		newLimit = usage + memoryBuffer

		if limit >= usage {
			// for memory cgroup, if usage value is nearly to limit, it may be hang for sometimes, so we add the buffer
			reason = fmt.Sprintf("mem cgroup limit value(%d) is nearly to usage(%d), add buffer: %d",
				limit, usage, newLimit)
		} else {
			// memory setting will failed when limited value is smaller than current usage.
			// there is no better way except dropping cache in cgroup level.
			// Now we just try to set limit value as current usage
			reason = fmt.Sprintf("mem cgroup limit value(%d) is less than current usage(%d),"+
				"try to set current value added buffer: %d", limit, usage, newLimit)
		}
	} else {
		klog.V(5).Infof("mem cgroup limit value(%d) is bigger than usage(%d)", limit, usage)
	}

	return newLimit, reason
}

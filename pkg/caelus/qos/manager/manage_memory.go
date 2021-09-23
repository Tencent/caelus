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
	"github.com/tencent/caelus/pkg/caelus/statestore"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/cgroup"
	"github.com/tencent/caelus/pkg/caelus/util/machine"

	"k8s.io/api/core/v1"
	"k8s.io/klog"
)

const QosMemory = "memory"

type qosMemory struct {
	stStore statestore.StateStore
}

// NewQosMemory creates memory manager instance
func NewQosMemory(stStore statestore.StateStore) ResourceQosManager {
	return &qosMemory{
		stStore: stStore,
	}
}

// Name returns resource policy name
func (m *qosMemory) Name() string {
	return QosMemory
}

// PreInit do nothing
func (m *qosMemory) PreInit() error {
	return nil
}

// Run starts nothing
func (m *qosMemory) Run(stop <-chan struct{}) {}

// ManageMemory isolates memory resource for offline jobs
func (m *qosMemory) Manage(cgResources *CgroupResourceConfig) error {
	quantity, ok := cgResources.Resources[v1.ResourceMemory]
	if !ok {
		klog.Warningf("no memory resource found for isolation")
		return nil
	}
	limit := quantity.Value()

	var usage, newLimit int64
	offlineParent := types.CgroupOffline
	state, err := m.stStore.GetCgroupResourceRecentStateByPath(offlineParent, false)
	if err != nil {
		// just warning, no need to return
		klog.Errorf("get cgroup(%s) memory usage err: %v", offlineParent, err)
		newLimit = limit
	} else {
		usage = int64(state.MemoryTotalUsage)
		var reason string
		newLimit, reason = machine.GetMemoryCgroupLimitByUsage(limit, usage)
		if len(reason) != 0 {
			klog.Warningf("mem cgroup(%s) limit setting changed: %s", offlineParent, reason)
		}
		klog.V(4).Infof("isolating memory for offline jobs, origin limit:%d, current usage:%d, new limit:%d",
			limit, usage, newLimit)
	}

	err = cgroup.SetMemoryLimit(offlineParent, newLimit)
	if err != nil {
		klog.Errorf("set memory cgroup(%s) offline limit value(%d, %d) failed: %v",
			offlineParent, newLimit, usage, err)
	}

	return err
}

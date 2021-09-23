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

package predict

import (
	"testing"
	"time"

	"github.com/tencent/caelus/pkg/caelus/statestore/cgroup"
	"github.com/tencent/caelus/pkg/caelus/statestore/common/node"
	mockStore "github.com/tencent/caelus/pkg/caelus/statestore/mock"
	"github.com/tencent/caelus/pkg/caelus/types"
	"github.com/tencent/caelus/pkg/caelus/util/appclass"
)

func TestLocalPredictGet(t *testing.T) {
	gb := float64(1024 * 1024 * 1024)
	now := time.Now()
	mockCgrouprStats := &mockStore.MockCgroupStore{
		CgStats: map[string]*cgroupstore.CgroupStats{
			"c1": {
				Ref:                   &cgroupstore.CgroupRef{Name: "c1", AppClass: appclass.AppClassOnline},
				CpuUsage:              1,
				MemoryWorkingSetUsage: 1 * gb,
				Timestamp:             now,
			},
			"c2": {
				Ref:                   &cgroupstore.CgroupRef{Name: "c2", AppClass: appclass.AppClassSystem},
				CpuUsage:              2,
				MemoryWorkingSetUsage: 3 * gb,
				Timestamp:             now,
			},
			// c3 won't be counted as it is a offline pod
			"c3": {
				Ref: &cgroupstore.CgroupRef{
					Name: "c2", PodName: "p1",
					AppClass: appclass.AppClassOffline},
				CpuUsage:              3,
				MemoryWorkingSetUsage: 5 * gb,
				Timestamp:             now,
			},
		},
	}
	mockCommonStore := &mockStore.MockCommonStore{
		MockNodeStore: &mockStore.MockNodeStore{
			Resources: []*nodestore.NodeResourceState{
				{
					Timestamp: now,
					CPU: &nodestore.NodeCpu{
						CpuTotal: 7, // suppose kernel+kubelet+docker used 1 core
					},
					Memory: &nodestore.NodeMemory{
						UsageRss: 11 * gb, // suppose kernel+kubelet+docker used 2 gb
					},
				},
			},
		},
	}

	mockStatStore := mockStore.NewMockStatStore(mockCommonStore, mockCgrouprStats)
	config := types.PredictConfig{PredictType: types.LocalPredictorType}
	types.InitPredictConfig(&config)
	localPredict := NewLocalPredict(config, mockStatStore).(*localPredict)
	localPredict.initSampleTimes = 1
	localPredict.addSample()
	rl := localPredict.Predict()
	if len(rl) == 0 {
		t.Fatal()
	}
	// cpu should be 4*1.15 safetyMarginFraction
	if rl.Cpu().MilliValue() < 4000 || rl.Cpu().MilliValue() > 5000 {
		t.Fatal(rl.Cpu().MilliValue())
	}
	// mem should be 6*1.15
	mem := rl.Memory().Value() / int64(gb)
	if mem < 5 || mem > 6 {
		t.Fatal(mem)
	}
}

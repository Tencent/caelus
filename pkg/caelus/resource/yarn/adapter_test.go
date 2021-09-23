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

package yarn

import (
	"testing"

	"github.com/tencent/caelus/pkg/caelus/types"
	global "github.com/tencent/caelus/pkg/types"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestRoundOffAdapter_ResourceAdapt test round off adapter
func TestRoundOffAdapter_ResourceAdapt(t *testing.T) {
	ginit := newFakeGinit(&global.NMCapacity{
		Vcores:    2,
		Millcores: 2000,
		MemoryMB:  1024,
	})

	resRoundoff := types.RoundOffResource{
		CPUMilli: 2000,
		MemMB:    512,
	}
	targetRes := v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(5600, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(4596*types.MemUnit, resource.DecimalSI),
	}
	expectRes := v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(4000, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(4096*types.MemUnit, resource.DecimalSI),
	}

	adapter := NewRoundOffAdapter(resRoundoff, ginit)
	reachMin := adapter.ResourceAdapt(targetRes)
	if reachMin {
		t.Fatalf("test reserve adapter err, should not reach to the min value")
	}
	if !targetRes.Cpu().Equal(expectRes.Cpu().DeepCopy()) ||
		!targetRes.Memory().Equal(expectRes.Memory().DeepCopy()) {
		t.Fatalf("test round off adapter, expect %+v, got %+v", expectRes, targetRes)
	}
}

type compareMinTestData struct {
	describe string
	vcores   int64
	memMb    int64
	expect   struct {
		vcores   int64
		memMb    int64
		reachMin bool
	}
}

// TestCompareAndReplaceMinCapacity test comparing min capacity
func TestCompareAndReplaceMinCapacity(t *testing.T) {
	ginit := newFakeGinit(&global.NMCapacity{
		Vcores:    3,
		Millcores: 3000,
		MemoryMB:  2048,
	})

	testCases := []compareMinTestData{
		{
			describe: "big than min",
			vcores:   5,
			memMb:    4096,
			expect: struct {
				vcores   int64
				memMb    int64
				reachMin bool
			}{vcores: 5, memMb: 4096, reachMin: false},
		},
		{
			describe: "little than min",
			vcores:   2,
			memMb:    1024,
			expect: struct {
				vcores   int64
				memMb    int64
				reachMin bool
			}{vcores: 3, memMb: 2048, reachMin: true},
		},
	}

	for _, tc := range testCases {
		targetRes := v1.ResourceList{
			v1.ResourceCPU:    *resource.NewMilliQuantity(tc.vcores*types.CpuUnit, resource.DecimalSI),
			v1.ResourceMemory: *resource.NewQuantity(tc.memMb*types.MemUnit, resource.DecimalSI),
		}
		expectRes := v1.ResourceList{
			v1.ResourceCPU:    *resource.NewMilliQuantity(tc.expect.vcores*types.CpuUnit, resource.DecimalSI),
			v1.ResourceMemory: *resource.NewQuantity(tc.expect.memMb*types.MemUnit, resource.DecimalSI),
		}

		reachMin := compareAndReplaceMinCapacity(targetRes, ginit.GetMinCapacity)
		if reachMin != tc.expect.reachMin {
			t.Fatalf("compare min capacity test case(%s) failed, expect reach min value: %v, got %v",
				tc.describe, tc.expect.reachMin, reachMin)
		}

		if !targetRes.Cpu().Equal(expectRes.Cpu().DeepCopy()) ||
			!targetRes.Memory().Equal(expectRes.Memory().DeepCopy()) {
			t.Fatalf("compare min capacity test case(%s) failed,, expect %+v, got %+v",
				tc.describe, expectRes, targetRes)
		}
	}
}

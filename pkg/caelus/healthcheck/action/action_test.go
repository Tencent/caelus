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

package action

import (
	"gotest.tools/assert"
	"testing"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// TestActionResourceMerge test action resource merge function
func TestActionResourceMerge(t *testing.T) {
	cases := []struct {
		a ActionResource
		b ActionResource
		r ActionResource
	}{
		{
			a: ActionResource{
				Conflicting: false,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaStep: resource.MustParse("5"),
				}},
			b: ActionResource{
				Conflicting: false,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaStep: resource.MustParse("4"),
				}},
			r: ActionResource{
				Conflicting: false,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaStep: resource.MustParse("4"),
				}},
		},
		{
			a: ActionResource{
				Conflicting: false,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaStep: resource.MustParse("5"),
				}},
			b: ActionResource{
				Conflicting: true,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaStep: resource.MustParse("4"),
				}},
			r: ActionResource{
				Conflicting: true,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaStep: resource.MustParse("4"),
				}},
		},
		{
			a: ActionResource{
				Conflicting: true,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaStep:  resource.MustParse("5"),
					FormulaTotal: resource.MustParse("1"),
				}},
			b: ActionResource{
				Conflicting: true,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaPercent: resource.MustParse("20"),
				}},
			r: ActionResource{
				Conflicting: true,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaStep:    resource.MustParse("5"),
					FormulaPercent: resource.MustParse("20"),
					FormulaTotal:   resource.MustParse("1"),
				}},
		},
	}
	for _, c := range cases {
		r := c.a.MergeByLittle(c.b)
		assert.DeepEqual(t, c.r, r)
	}
}

// TestActionResultMerge test action result merge function
func TestActionResultMerge(t *testing.T) {
	merge := &ActionResult{
		UnscheduleMap: map[string]bool{"cpu": true, "osd:latency": true, "memory": false},
		EvictPods:     []k8stypes.NamespacedName{{Namespace: "ns1", Name: "pod-1"}},
		AdjustResources: map[v1.ResourceName]ActionResource{
			v1.ResourceCPU: {
				Conflicting: true,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaPercent: resource.MustParse("15"),
				}},
			v1.ResourceMemory: {
				Conflicting: false,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaPercent: resource.MustParse("20"),
				}},
		},
		SyncEvent: true,
	}
	b := &ActionResult{
		UnscheduleMap: map[string]bool{"cpu": true, "osd:latency": false, "memory": true, "slo": false},
		EvictPods:     []k8stypes.NamespacedName{{Namespace: "ns1", Name: "pod-1"}, {Namespace: "ns1", Name: "pod-2"}},
		AdjustResources: map[v1.ResourceName]ActionResource{
			v1.ResourceCPU: {
				Conflicting: true,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaPercent: resource.MustParse("10"),
				}},
		},
		SyncEvent: true,
	}
	expected := &ActionResult{
		UnscheduleMap: map[string]bool{"cpu": true, "osd:latency": true, "memory": true, "slo": false},
		EvictPods:     []k8stypes.NamespacedName{{Namespace: "ns1", Name: "pod-1"}, {Namespace: "ns1", Name: "pod-2"}},
		AdjustResources: map[v1.ResourceName]ActionResource{
			v1.ResourceCPU: {
				Conflicting: true,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaPercent: resource.MustParse("10"),
				}},
			v1.ResourceMemory: {
				Conflicting: false,
				ConflictQuantity: map[ActionFormula]resource.Quantity{
					FormulaPercent: resource.MustParse("20"),
				}},
		},
		SyncEvent: true,
	}

	merge.Merge(b)
	assert.DeepEqual(t, *expected, *merge)
}

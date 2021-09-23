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

package conflict

import (
	"gotest.tools/assert"
	"testing"

	"github.com/tencent/caelus/pkg/caelus/healthcheck/action"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestConflictManager test updating conflicting resource list
func TestConflictManager(t *testing.T) {
	m := NewConflictManager()
	adj1 := map[v1.ResourceName]action.ActionResource{
		v1.ResourceCPU + "_" + v1.ResourceMemory: {
			Name:        v1.ResourceCPU,
			Conflicting: false,
			ConflictQuantity: map[action.ActionFormula]resource.Quantity{
				action.FormulaStep: resource.MustParse("1"),
			},
		},
	}
	changed, _ := m.UpdateConflictList(adj1)
	if changed {
		t.Errorf("expect conflict resource not changed because no conflict")
	}
	adj1 = map[v1.ResourceName]action.ActionResource{
		v1.ResourceCPU + "_" + v1.ResourceMemory: {
			Name:        v1.ResourceCPU,
			Conflicting: true,
			ConflictQuantity: map[action.ActionFormula]resource.Quantity{
				action.FormulaStep: resource.MustParse("-1"),
			},
		},
	}
	changed, _ = m.UpdateConflictList(adj1)
	if !changed {
		t.Errorf("expect conflict resource change because of conflict")
	}
	changed, _ = m.UpdateConflictList(adj1)
	if !changed {
		t.Errorf("expect conflict resource change because of conflict")
	}
	adj1 = map[v1.ResourceName]action.ActionResource{
		v1.ResourceCPU + "_" + v1.ResourceMemory: {
			Name:        v1.ResourceCPU,
			Conflicting: true,
			ConflictQuantity: map[action.ActionFormula]resource.Quantity{
				action.FormulaPercent: resource.MustParse("-0.1"),
			},
		},
	}
	changed, _ = m.UpdateConflictList(adj1)
	if !changed {
		t.Errorf("expect conflict resource change because of conflict")
	}
	constPredictRes := v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("3"),
		v1.ResourceMemory: resource.MustParse("5Gi"),
	}
	predictList := constPredictRes.DeepCopy()
	confRes, _ := m.CheckAndSubConflictResource(predictList)
	assert.DeepEqual(t, confRes, map[v1.ResourceName]bool{v1.ResourceCPU: true})
	assert.DeepEqual(t, predictList[v1.ResourceCPU], resource.MustParse("1"))
	adj1 = map[v1.ResourceName]action.ActionResource{
		v1.ResourceCPU + "_" + v1.ResourceMemory: {
			Name:        v1.ResourceCPU,
			Conflicting: false,
			ConflictQuantity: map[action.ActionFormula]resource.Quantity{
				action.FormulaStep: resource.MustParse("1"),
			},
		},
	}
	_, _ = m.UpdateConflictList(adj1)
	predictList = constPredictRes.DeepCopy()
	confRes, _ = m.CheckAndSubConflictResource(predictList)
	assert.DeepEqual(t, confRes, map[v1.ResourceName]bool{v1.ResourceCPU: false})
	assert.DeepEqual(t, predictList[v1.ResourceCPU], resource.MustParse("2"))
	_, _ = m.UpdateConflictList(adj1)

	predictList = constPredictRes.DeepCopy()
	_, _ = m.CheckAndSubConflictResource(predictList)
	assert.DeepEqual(t, predictList[v1.ResourceCPU], resource.MustParse("2.7"))
}

var mergeConflictResourceCases = []struct {
	describe                 string
	conflictingResources     map[v1.ResourceName]action.ActionResource
	expectMergedResourceList map[v1.ResourceName]resource.Quantity
	expectMergedConflicting  map[v1.ResourceName]bool
}{
	{
		describe: "step test",
		conflictingResources: map[v1.ResourceName]action.ActionResource{
			v1.ResourceCPU: {
				Name:        v1.ResourceCPU,
				Conflicting: true,
				ConflictQuantity: map[action.ActionFormula]resource.Quantity{
					action.FormulaStep: resource.MustParse("-1000"),
				},
			},
		},
		expectMergedResourceList: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU: *resource.NewQuantity(-1000, resource.DecimalSI),
		},
		expectMergedConflicting: map[v1.ResourceName]bool{
			v1.ResourceCPU: true,
		},
	},
	{
		describe: "percent test",
		conflictingResources: map[v1.ResourceName]action.ActionResource{
			v1.ResourceCPU: {
				Name:        v1.ResourceCPU,
				Conflicting: true,
				ConflictQuantity: map[action.ActionFormula]resource.Quantity{
					action.FormulaPercent: *resource.NewMilliQuantity(-500, resource.DecimalSI),
				},
			},
		},
		expectMergedResourceList: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU: *resource.NewQuantity(-500, resource.DecimalSI),
		},
		expectMergedConflicting: map[v1.ResourceName]bool{
			v1.ResourceCPU: true,
		},
	},
	{
		describe: "resource name translating test",
		conflictingResources: map[v1.ResourceName]action.ActionResource{
			v1.ResourceCPU: {
				Name:        v1.ResourceCPU,
				Conflicting: true,
				ConflictQuantity: map[action.ActionFormula]resource.Quantity{
					action.FormulaStep: resource.MustParse("-300"),
				},
			},
			"diskio": {
				Name:        v1.ResourceCPU,
				Conflicting: true,
				ConflictQuantity: map[action.ActionFormula]resource.Quantity{
					action.FormulaPercent: *resource.NewMilliQuantity(-500, resource.DecimalSI),
				},
			},
		},
		expectMergedResourceList: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU: *resource.NewQuantity(-500, resource.DecimalSI),
		},
		expectMergedConflicting: map[v1.ResourceName]bool{
			v1.ResourceCPU: true,
		},
	},
}

// TestMergeConflictResource test merge conflicting resources
func TestMergeConflictResource(t *testing.T) {
	predictList := v1.ResourceList{
		v1.ResourceCPU: *resource.NewQuantity(1000, resource.DecimalSI),
	}
	for _, tCase := range mergeConflictResourceCases {
		t.Logf("merge conflicting resources testing: %s", tCase.describe)
		cm := &manager{
			conflictingResources: tCase.conflictingResources,
		}

		mergedResourceList, mergedConflicting := cm.mergeConflictResource(predictList)
		if !mergedResultEqual(mergedResourceList, mergedConflicting,
			tCase.expectMergedResourceList, tCase.expectMergedConflicting) {
			t.Fatalf("merge conflicting resources test case(%s) fail,  got "+
				"resourceList: %v, conflictingList: %v, expect resourceList: %v, "+
				"conflictingList: %v", tCase.describe, mergedResourceList, mergedConflicting,
				tCase.expectMergedResourceList, tCase.expectMergedConflicting)
		}
	}
}

func mergedResultEqual(resList map[v1.ResourceName]resource.Quantity, conflictingList map[v1.ResourceName]bool,
	expResList map[v1.ResourceName]resource.Quantity, expConflictingList map[v1.ResourceName]bool) bool {
	if len(resList) != len(expResList) ||
		len(conflictingList) != len(expConflictingList) {
		return false
	}

	for k, v := range resList {
		if vv, ok := expResList[k]; !ok {
			return false
		} else if !v.Equal(vv) {
			return false
		}

		mC, okM := conflictingList[k]
		eC, okC := expConflictingList[k]
		if !okM || !okC {
			return false
		}
		if mC != eC {
			return false
		}
	}

	return true
}

func TestQuickRecoverConflictResource(t *testing.T) {
	m := NewConflictManager()
	adj1 := map[v1.ResourceName]action.ActionResource{
		v1.ResourceCPU + "_" + v1.ResourceMemory: {
			Name:        v1.ResourceCPU,
			Conflicting: true,
			ConflictQuantity: map[action.ActionFormula]resource.Quantity{
				action.FormulaStep: resource.MustParse("-5"),
			},
		},
	}
	_, _ = m.UpdateConflictList(adj1)
	predictList := v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("3"),
		v1.ResourceMemory: resource.MustParse("5Gi"),
	}
	confRes, _ := m.CheckAndSubConflictResource(predictList)
	assert.DeepEqual(t, confRes, map[v1.ResourceName]bool{v1.ResourceCPU: true})
	assert.DeepEqual(t, predictList[v1.ResourceCPU], resource.MustParse("0"))
	predictList = v1.ResourceList{
		v1.ResourceCPU:    resource.MustParse("4"),
		v1.ResourceMemory: resource.MustParse("5Gi"),
	}
	confRes, _ = m.CheckAndSubConflictResource(predictList)
	assert.DeepEqual(t, confRes, map[v1.ResourceName]bool{v1.ResourceCPU: true})
	assert.DeepEqual(t, predictList[v1.ResourceCPU], resource.MustParse("1"))
}
